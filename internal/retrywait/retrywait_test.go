package retrywait

import (
	"errors"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func seconds(v float64) *float64 { return &v }

// Every loop waits at least the delay the answer asked for, read from any SDK
// error, an authentication refusal included, or a ConnectRPC answer's headers.
func TestDelayReadsTheAnswersRetryAfter(t *testing.T) {
	withHeader := connect.NewError(connect.CodeUnavailable, errors.New("busy"))
	withHeader.Meta().Set("Retry-After", "3")
	for _, tc := range []struct {
		name string
		err  error
		want time.Duration
	}{
		{"none", &sdkerrors.ServerError{Msg: "x"}, 25 * time.Millisecond},
		{"server", &sdkerrors.ServerError{RetryAfter: seconds(2)}, 2 * time.Second},
		{"auth unavailable", &sdkerrors.APIError{Code: sdkerrors.AuthCodeTemporarilyUnavailable, RetryAfter: seconds(30)}, 30 * time.Second},
		{"shorter than the fallback", &sdkerrors.APIError{RetryAfter: seconds(0.001)}, 25 * time.Millisecond},
		{"rate limit", &sdkerrors.RateLimitError{RetryAfter: seconds(1.5)}, 1500 * time.Millisecond},
		{"wrapped by connect", connect.NewError(connect.CodeUnavailable, &sdkerrors.APIError{RetryAfter: seconds(4)}), 4 * time.Second},
		{"connect metadata", withHeader, 3 * time.Second},
		{"huge", &sdkerrors.ServerError{RetryAfter: seconds(math.MaxFloat64)}, maxWait},
	} {
		if got := Delay(tc.err, 25*time.Millisecond); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
