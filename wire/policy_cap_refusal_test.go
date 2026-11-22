package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

// A sub-account policy cap refusal (POLICY_MAX_NOTIONAL, POLICY_MAX_OPEN_ORDERS)
// is invalid_argument (HTTP 400): an order refusal, not a credentials
// problem. It maps by its status to an *errors.APIError that keeps its code,
// never to an *errors.AuthError, whatever "POLICY" its name contains.
func TestPolicyCapRefusalIsAnAPIErrorNotAnAuthError(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_POLICY_MAX_NOTIONAL,
		orderv1.ErrorCode_ERROR_CODE_POLICY_MAX_OPEN_ORDERS,
	} {
		t.Run(code.String(), func(t *testing.T) {
			mapped := mapOrderRefusal(t, code, connect.CodeInvalidArgument)
			var authErr *sdkerrors.AuthError
			if errors.As(mapped, &authErr) {
				t.Fatalf("mapped to an AuthError: %#v", mapped)
			}
			var apiErr *sdkerrors.APIError
			if !errors.As(mapped, &apiErr) || apiErr.Code != code.String() {
				t.Fatalf("mapped=%T %#v, want an APIError with code %s", mapped, mapped, code)
			}
			if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("%s must be final with a known outcome", code)
			}
		})
	}
}

// A policy code with permission_denied (or unauthenticated) is still a
// permission refusal: an *errors.AuthError carrying its code.
func TestPolicyPermissionRefusalStaysAnAuthError(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_POLICY_SPOT_TRADE_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_MARKET_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_SPOT_READ_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_TRADING_HALTED,
	} {
		for _, status := range []connect.Code{connect.CodePermissionDenied, connect.CodeUnauthenticated} {
			t.Run(code.String()+"/"+status.String(), func(t *testing.T) {
				mapped := mapOrderRefusal(t, code, status)
				var authErr *sdkerrors.AuthError
				if !errors.As(mapped, &authErr) || authErr.Code != code.String() {
					t.Fatalf("mapped=%T %#v, want an AuthError with code %s", mapped, mapped, code)
				}
			})
		}
	}
}
