package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
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
