package services

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// A retryable answer that arrives as the caller's context ends would have been
// retried; only the caller ended the read, so it ends with the caller's error.
func TestRetryUnavailableAnswerToAnEndedCallerIsTheCallersError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer connect.Code
		end    func() (context.Context, context.CancelFunc, func())
		want   error
	}{
		{
			name:   "unavailable then cancel",
			answer: connect.CodeUnavailable,
			end: func() (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, cancel, cancel
			},
			want: context.Canceled,
		},
		{
			name:   "deadline exceeded answer then cancel",
			answer: connect.CodeDeadlineExceeded,
			end: func() (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, cancel, cancel
			},
			want: context.Canceled,
		},
		{
			name:   "unavailable past the caller deadline",
			answer: connect.CodeUnavailable,
			end: func() (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				return ctx, cancel, func() {}
			},
			want: context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel, end := tc.end()
			defer cancel()
			calls := 0
			read := retryUnavailable(triggerReadRetry, func(context.Context, *connect.Request[triggerv1.GetTriggerRequest]) (*connect.Response[triggerv1.GetTriggerResponse], error) {
				calls++
				// The answer is in; the caller ends before the retry decides.
				end()
				return nil, connect.NewError(tc.answer, errors.New("catching up"))
			})
			_, err := read(ctx, connect.NewRequest(&triggerv1.GetTriggerRequest{}))
			err = transport.MapError(err)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%T %v, want errors.Is(err, %v)", err, err, tc.want)
			}
			var transportErr *sdkerrors.TransportError
			if !errors.As(err, &transportErr) {
				t.Fatalf("err=%T %v, want *TransportError", err, err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d, want 1: an ended caller's read must stop retrying", calls)
			}
		})
	}
}

// A definite answer stays the answer even when the caller has ended.
func TestRetryUnavailableKeepsADefiniteAnswerToAnEndedCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := retryUnavailable(triggerReadRetry, func(context.Context, *connect.Request[triggerv1.GetTriggerRequest]) (*connect.Response[triggerv1.GetTriggerResponse], error) {
		cancel()
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such trigger"))
	})
	_, err := read(ctx, connect.NewRequest(&triggerv1.GetTriggerRequest{}))
	if connect.CodeOf(err) != connect.CodeNotFound || errors.Is(err, context.Canceled) {
		t.Fatalf("err=%T %v, want the not-found answer", err, err)
	}
}

// A read retried after a temporary failure waits at least the Retry-After
// its answer asked for, and gives up at once when that would end past the
// retry budget. The answer arrives through the real transport.
func TestRetryUnavailableHonoursRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		retryAfter string
		wantCalls  int32
		// wantWaits is the one wait before the retry; none past the budget.
		wantWaits []time.Duration
	}{
		{"past the budget", `{"code":"UPSTREAM_ERROR","status":503}`, "30", 1, nil},
		{"within the budget", `{"code":"UPSTREAM_ERROR","status":503}`, "0.4", 2, []time.Duration{400 * time.Millisecond}},
		{"auth unavailable past the budget", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`, "30", 1, nil},
		{"auth unavailable within the budget", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`, "0.4", 2, []time.Duration{400 * time.Millisecond}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := useFakeClock(t)
			var calls atomic.Int32
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Content-Type", "application/problem+json")
					w.Header().Set("Retry-After", tc.retryAfter)
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				w.Header().Set("Content-Type", "application/proto")
				_, _ = w.Write(nil)
			}))
			t.Cleanup(srv.Close)
			factory := transport.NewFactory(transport.Config{APIURL: srv.URL, Timeout: 10 * time.Second}, nil, nil)
			client := triggersv1connect.NewTriggersServiceClient(factory.HTTP, srv.URL, factory.ConnectOptions(false)...)
			_, err := retryUnavailable(triggerReadRetry, client.GetTrigger)(context.Background(), connect.NewRequest(&triggerv1.GetTriggerRequest{}))
			if waits := clock.Waits(); calls.Load() != tc.wantCalls || !slices.Equal(waits, tc.wantWaits) {
				t.Fatalf("calls=%d waits=%v err=%v, want %d calls after %v", calls.Load(), waits, err, tc.wantCalls, tc.wantWaits)
			}
			if tc.wantCalls == 1 {
				if mapped := transport.MapError(err); sdkerrors.RetryAfter(mapped) == nil || *sdkerrors.RetryAfter(mapped) != 30 {
					t.Fatalf("err=%T %+v", mapped, mapped)
				}
			} else if err != nil {
				t.Fatalf("second attempt: %v", err)
			}
		})
	}
}

// Round 4 controls: a read retried after any of these answers waits for its
// Retry-After (0.4 s), and returns at once with the delay on its error when
// the delay is past the budget (30 s). A refusal is never retried.
func TestRetryUnavailableHonoursRetryAfterOnEveryAnswer(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		retried     bool
	}{
		{"deadline 504", 504, "application/json", `{"code":"deadline_exceeded","message":"context deadline exceeded"}`, true},
		{"auth 503", 503, "application/problem+json", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`, true},
		{"permission denied 403", 403, "application/problem+json", `{"code":"PERMISSION_DENIED","status":403,"detail":"denied"}`, false},
		{"malformed message 503", 503, "application/json", `{"code":"unavailable","message":5}`, true},
		{"malformed message 504", 504, "application/json", `{"code":"unavailable","message":5}`, true},
		{"malformed detail 503", 503, "application/json", `{"code":"unavailable","message":"m","details":"nope"}`, true},
		{"malformed detail 504", 504, "application/json", `{"code":"unavailable","message":"m","details":[{"type":"orders.v1.ErrorDetail","value":"!!"}]}`, true},
	} {
		for _, delay := range []string{"30", "0.4"} {
			t.Run(tc.name+"/"+delay, func(t *testing.T) {
				clock := useFakeClock(t)
				var calls atomic.Int32
				srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if calls.Add(1) == 1 {
						w.Header().Set("Content-Type", tc.contentType)
						w.Header().Set("Retry-After", delay)
						w.WriteHeader(tc.status)
						_, _ = w.Write([]byte(tc.body))
						return
					}
					w.Header().Set("Content-Type", "application/proto")
					_, _ = w.Write(nil)
				}))
				t.Cleanup(srv.Close)
				factory := transport.NewFactory(transport.Config{APIURL: srv.URL, Timeout: 10 * time.Second}, nil, nil)
				client := triggersv1connect.NewTriggersServiceClient(factory.HTTP, srv.URL, factory.ConnectOptions(false)...)
				_, err := retryUnavailable(triggerReadRetry, client.GetTrigger)(context.Background(), connect.NewRequest(&triggerv1.GetTriggerRequest{}))
				waits := clock.Waits()
				if !tc.retried || delay == "30" {
					if calls.Load() != 1 || len(waits) != 0 {
						t.Fatalf("calls=%d waits=%v, want one answer returned at once", calls.Load(), waits)
					}
					want := 30.0
					if delay == "0.4" {
						want = 0.4
					}
					if got := sdkerrors.RetryAfter(transport.MapError(err)); got == nil || *got != want {
						t.Fatalf("err=%T %v retry after %v, want %v", err, err, got, want)
					}
					return
				}
				if err != nil || calls.Load() != 2 || !slices.Equal(waits, []time.Duration{400 * time.Millisecond}) {
					t.Fatalf("calls=%d waits=%v err=%v, want a retry after 0.4 s", calls.Load(), waits, err)
				}
			})
		}
	}
}
