package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
)

// A read refused under the trading read limit is a rate refusal like a
// placement or cancellation one, and names its class.
func TestTradingReadRateRefusalNamesItsClass(t *testing.T) {
	retryAfterMs := uint64(500)
	connectErr := connect.NewError(connect.CodeResourceExhausted, nil)
	detail, err := connect.NewErrorDetail(&ratelimitv1.RateLimitDetail{
		Reason:       ratelimitv1.FailureReason_QUOTA_EXCEEDED,
		RetryAfterMs: &retryAfterMs,
		PolicyClass:  ratelimitv1.PolicyClass_TRADING_READ,
	})
	if err != nil {
		t.Fatalf("NewErrorDetail: %v", err)
	}
	connectErr.AddDetail(detail)
	var rate *sdkerrors.RateLimitError
	if !errors.As(MapConnectError(connectErr), &rate) {
		t.Fatalf("mapped=%T", MapConnectError(connectErr))
	}
	if rate.Detail == nil || rate.Detail.PolicyClass != "TRADING_READ" || rate.RetryAfter == nil || *rate.RetryAfter != 0.5 {
		t.Fatalf("detail=%#v retry=%v", rate.Detail, rate.RetryAfter)
	}
}
