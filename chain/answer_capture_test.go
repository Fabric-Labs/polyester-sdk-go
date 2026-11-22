package chain

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
)

func countingRPC(t *testing.T, answer http.HandlerFunc) (string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

func timedRequest(url string) (time.Duration, error) {
	started := time.Now()
	_, err := NewJSONRPCClient(url, 400*time.Millisecond).Request("eth_chainId", nil)
	return time.Since(started), err
}

func expectAnswer(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatal("no error")
	}
	if got := sdkerrors.AnswerStatus(err); got != status {
		t.Errorf("%T %v: status %d, want %d", err, err, got, status)
	}
	if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
		t.Errorf("%T %v: retry after %v, want 30", err, err, got)
	}
}

// A JSON-RPC answer's status and delay are fixed when its headers arrive, so
// a failure reading its body still carries them.
func TestJSONRPCBodyFailuresCarryTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer http.HandlerFunc
	}{
		{"truncated", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{"))
		}},
		{"oversized", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write(make([]byte, MaxJSONRPCResponseBytes+16))
		}},
		{"body deadline", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{"))
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url, calls := countingRPC(t, tc.answer)
			took, err := timedRequest(url)
			if calls.Load() != 1 || took > 3*time.Second {
				t.Errorf("%d requests in %v", calls.Load(), took)
			}
			expectAnswer(t, err, http.StatusServiceUnavailable)
		})
	}
}

// A JSON-RPC redirect is never followed: its status and delay ride on the
// error, whether its Location parses or not, and the target receives nothing.
func TestJSONRPCRedirectsCarryTheAnswer(t *testing.T) {
	target, targetCalls := countingRPC(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	})
	for _, tc := range []struct{ name, location string }{
		{"valid Location", target + "/elsewhere"},
		{"malformed Location", "http://[::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url, calls := countingRPC(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "30")
				w.Header().Set("Location", tc.location)
				w.WriteHeader(http.StatusTemporaryRedirect)
			})
			_, err := timedRequest(url)
			if calls.Load() != 1 || targetCalls.Load() != 0 {
				t.Errorf("%d requests, %d at the target", calls.Load(), targetCalls.Load())
			}
			expectAnswer(t, err, http.StatusTemporaryRedirect)
		})
	}
}

// A refused connection has no answer to carry.
func TestJSONRPCRefusedConnectionCarriesNoAnswer(t *testing.T) {
	_, err := timedRequest("http://127.0.0.1:1")
	if err == nil || sdkerrors.AnswerStatus(err) != 0 || sdkerrors.RetryAfter(err) != nil {
		t.Fatalf("%T %v", err, err)
	}
}
