package transport

import (
	"errors"
	"fmt"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestRESTOrderUnavailableClassification(t *testing.T) {
	for _, code := range []string{
		"RATE_LIMIT_POLICY_UNAVAILABLE", "POLICY_UNAVAILABLE",
		"FEE_SCHEDULE_UNAVAILABLE", "UNAVAILABLE",
	} {
		err := callMeAgainst(t, 503, "application/problem+json",
			fmt.Sprintf("{\"code\":%q,\"status\":503,\"detail\":\"retry later\"}", code))
		var server *sdkerrors.ServerError
		var auth *sdkerrors.AuthError
		if !errors.As(err, &server) || errors.As(err, &auth) || !sdkerrors.IsRetryable(err) {
			t.Fatalf("%s mapped=%T retryable=%v: %v", code, err, sdkerrors.IsRetryable(err), err)
		}
	}
}
