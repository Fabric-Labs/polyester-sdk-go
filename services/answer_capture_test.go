package services

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// capturedTriggers is a signed TriggersService against apiURL whose requests
// time out after 400 ms.
func capturedTriggers(t *testing.T, apiURL string) *TriggersService {
	t.Helper()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = apiURL
	cfg.Timeout = 400 * time.Millisecond
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

// answerAt serves every request with answer and counts them.
func answerAt(t *testing.T, answer http.HandlerFunc) (string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

// getTrigger reads a trigger once, as a caller does, and times it.
// getTrigger reads a trigger on the fake clock and returns the read
// retry's waits with the error: the retry budget counts those waits, not how
// fast the machine answers.
func getTrigger(t *testing.T, apiURL string) ([]time.Duration, error) {
	t.Helper()
	clock := useFakeClock(t)
	service := capturedTriggers(t, apiURL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := service.Get(ctx, nil, "7", nil)
	return clock.Waits(), err
}

// The answer's status and delay are fixed when its headers arrive, so a
// failure reading its body still carries them: one request, an immediate
// return (the 30 s delay is past the read's retry budget), 503 and 30.
func TestBodyFailuresCarryTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer http.HandlerFunc
	}{
		{"truncated", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{"))
		}},
		{"oversized", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"unavailable","message":"` + strings.Repeat("x", 256*1024) + `"}`))
		}},
		{"body deadline", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "application/json")
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
			apiURL, calls := answerAt(t, tc.answer)
			waits, err := getTrigger(t, apiURL)
			if err == nil {
				t.Fatal("no error")
			}
			if calls.Load() != 1 || len(waits) != 0 {
				t.Errorf("%d requests after waits %v, want one and no wait", calls.Load(), waits)
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

// A redirect is never followed: its status and delay ride on the refusal,
// whether its Location parses or not, and the target receives nothing.
func TestRefusedRedirectsCarryTheAnswer(t *testing.T) {
	target, targetCalls := answerAt(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range []struct {
		name, location string
		malformed      bool
	}{
		{"valid Location", target + "/elsewhere", false},
		{"malformed Location", "http://[::1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiURL, calls := answerAt(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "30")
				w.Header().Set("Location", tc.location)
				w.WriteHeader(http.StatusTemporaryRedirect)
			})
			waits, err := getTrigger(t, apiURL)
			var refused *sdkerrors.RedirectRefusedError
			if !errors.As(err, &refused) || refused.Malformed != tc.malformed {
				t.Fatalf("%T %v, want a refused redirect (malformed %v)", err, err, tc.malformed)
			}
			if calls.Load() != 1 || targetCalls.Load() != 0 || len(waits) != 0 {
				t.Errorf("%d requests, %d at the target, after waits %v", calls.Load(), targetCalls.Load(), waits)
			}
			if got := sdkerrors.AnswerStatus(err); got != http.StatusTemporaryRedirect {
				t.Errorf("status %d, want 307", got)
			}
			if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
				t.Errorf("retry after %v, want 30", got)
			}
		})
	}
}

// A redirect with nothing to follow is an answer too: it carries its status
// and delay rather than reaching the caller as a bare protocol error.
func TestUnfollowableRedirectCarriesTheAnswer(t *testing.T) {
	apiURL, calls := answerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	_, err := getTrigger(t, apiURL)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("%v after %d requests", err, calls.Load())
	}
	if got := sdkerrors.AnswerStatus(err); got != http.StatusTemporaryRedirect {
		t.Errorf("%T %v: status %d, want 307", err, err, got)
	}
	if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
		t.Errorf("retry after %v, want 30", got)
	}
}

// A refused connection has no answer, so its error carries none.
func TestRefusedConnectionCarriesNoAnswer(t *testing.T) {
	waits, err := getTrigger(t, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("no error")
	}
	// A refused connection is retried within the budget: 25 ms doubling to
	// 500 ms, seven waits.
	if want := triggerReadBackoff(8); !slices.Equal(waits, want) {
		t.Fatalf("waits=%v, want %v", waits, want)
	}
	if sdkerrors.AnswerStatus(err) != 0 || sdkerrors.RetryAfter(err) != nil {
		t.Fatalf("%T %v carries an answer", err, err)
	}
}
