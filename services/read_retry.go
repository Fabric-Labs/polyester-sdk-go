package services

import (
	"context"
	"time"

	"connectrpc.com/connect"
)

// unavailableRetry retries a side-effect-free read that answered UNAVAILABLE
// (Connect unavailable, or HTTP 503), with the same request. The first wait
// is first, each next one doubles up to max, and no wait starts that would
// end after budget from the first attempt; the last error is then returned.
type unavailableRetry struct {
	first  time.Duration
	max    time.Duration
	budget time.Duration
}

// triggerReadRetry is the default retry of GetTrigger, ListTriggers and ListTriggerEvents. Once
// ModifyTrigger, PauseTrigger or ResumeTrigger has answered, these reads
// return the trigger as changed, or UNAVAILABLE until they can.
var triggerReadRetry = unavailableRetry{
	first:  25 * time.Millisecond,
	max:    500 * time.Millisecond,
	budget: 2 * time.Second,
}

func retryUnavailable[Req any, Resp any](
	policy unavailableRetry,
	call func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
) func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error) {
	return func(ctx context.Context, req *connect.Request[Req]) (*connect.Response[Resp], error) {
		header := req.Header().Clone()
		started := time.Now()
		delay := policy.first
		attempt := req
		for {
			resp, err := call(ctx, attempt)
			if err == nil || (connect.CodeOf(err) != connect.CodeUnavailable && connect.CodeOf(err) != connect.CodeDeadlineExceeded) {
				return resp, err
			}
			if ctx.Err() != nil {
				// The answer would be retried; only the caller ended the read.
				return nil, ctx.Err()
			}
			if time.Since(started)+delay > policy.budget {
				return nil, err
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			delay = min(2*delay, policy.max)
			// Each attempt signs a fresh request carrying the same message.
			attempt = connect.NewRequest(req.Msg)
			for name, values := range header {
				attempt.Header()[name] = values
			}
		}
	}
}
