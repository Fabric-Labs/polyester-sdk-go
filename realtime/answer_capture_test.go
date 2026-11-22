package realtime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
)

// upgradingServer accepts the realtime WebSocket and waits, so a private
// subscription goes on to its token request.
func upgradingServer(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{Subprotocols: []string{centrifugoProtobufSubprotocol}}
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// countingServer serves every request with answer and counts them.
func countingServer(t *testing.T, answer http.HandlerFunc) (string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

// subscribePrivate subscribes to a private channel once, as a caller does,
// with the token requests going to apiURL, and times it.
func subscribePrivate(t *testing.T, apiURL string) (time.Duration, error) {
	t.Helper()
	client := NewClient(upgradingServer(t), apiURL, testCreds(t), &http.Client{Timeout: 400 * time.Millisecond}, 200)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	started := time.Now()
	sub, err := SubscribeProto(ctx, client, "private:orders:1", func(data []byte) ([]byte, error) { return data, nil })
	if sub != nil {
		sub.Close()
	}
	return time.Since(started), err
}

// A token answer's status and delay are fixed when its headers arrive, so a
// failure reading its body still carries them.
func TestTokenBodyFailuresCarryTheAnswer(t *testing.T) {
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
			_, _ = w.Write([]byte(strings.Repeat("x", 256*1024)))
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
			apiURL, calls := countingServer(t, tc.answer)
			took, err := subscribePrivate(t, apiURL)
			if err == nil {
				t.Fatal("no error")
			}
			if calls.Load() != 1 || took > 3*time.Second {
				t.Errorf("%d token requests in %v, want one and an immediate return", calls.Load(), took)
			}
			if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
				t.Errorf("%T %v: status %d, want 503", err, err, got)
			}
			if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
				t.Errorf("%T %v: retry after %v, want 30", err, err, got)
			}
		})
	}
}

// A redirect of the token request is never followed: its status and delay
// ride on the error whether its Location parses or not, or is missing, and
// the target receives nothing.
func TestTokenRedirectsCarryTheAnswer(t *testing.T) {
	target, targetCalls := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range []struct{ name, location string }{
		{"valid Location", target + "/elsewhere"},
		{"malformed Location", "http://[::1"},
		{"no Location", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiURL, calls := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "30")
				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(http.StatusTemporaryRedirect)
			})
			took, err := subscribePrivate(t, apiURL)
			if err == nil {
				t.Fatal("no error")
			}
			if calls.Load() != 1 || targetCalls.Load() != 0 || took > 3*time.Second {
				t.Errorf("%d requests, %d at the target, in %v", calls.Load(), targetCalls.Load(), took)
			}
			if got := sdkerrors.AnswerStatus(err); got != http.StatusTemporaryRedirect {
				t.Errorf("%T %v: status %d, want 307", err, err, got)
			}
			if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
				t.Errorf("%T %v: retry after %v, want 30", err, err, got)
			}
		})
	}
}

// A token request whose connection is refused has no answer to carry.
func TestTokenRefusedConnectionCarriesNoAnswer(t *testing.T) {
	_, err := subscribePrivate(t, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("no error")
	}
	var refused *sdkerrors.RedirectRefusedError
	if errors.As(err, &refused) || sdkerrors.AnswerStatus(err) != 0 || sdkerrors.RetryAfter(err) != nil {
		t.Fatalf("%T %v carries an answer", err, err)
	}
}
