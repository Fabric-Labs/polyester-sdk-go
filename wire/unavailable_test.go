package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestOrderUnavailableClassification(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_POLICY_UNAVAILABLE,
		orderv1.ErrorCode_ERROR_CODE_POLICY_UNAVAILABLE,
		orderv1.ErrorCode_ERROR_CODE_FEE_SCHEDULE_UNAVAILABLE,
		orderv1.ErrorCode_ERROR_CODE_UNAVAILABLE,
	} {
		t.Run(code.String(), func(t *testing.T) {
			err := connect.NewError(connect.CodeUnavailable, errors.New("retry later"))
			detail, detailErr := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: code})
			if detailErr != nil {
				t.Fatal(detailErr)
			}
			err.AddDetail(detail)
			mapped := MapConnectError(err)
			var server *sdkerrors.ServerError
			var auth *sdkerrors.AuthError
			if !errors.As(mapped, &server) || errors.As(mapped, &auth) || !sdkerrors.IsRetryable(mapped) {
				t.Fatalf("mapped=%T retryable=%v: %v", mapped, sdkerrors.IsRetryable(mapped), mapped)
			}
			if server.Code != code.String() || server.Status != 503 {
				t.Fatalf("code=%q status=%d", server.Code, server.Status)
			}
		})
	}
}

func TestOrderPolicyDenialClassification(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_POLICY_MARKET_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_TRADING_HALTED,
		orderv1.ErrorCode_ERROR_CODE_POLICY_SPOT_TRADE_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_SPOT_READ_DENY,
	} {
		err := connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
		detail, detailErr := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: code})
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		err.AddDetail(detail)
		mapped := MapConnectError(err)
		var auth *sdkerrors.AuthError
		if !errors.As(mapped, &auth) || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s mapped=%T retryable=%v", code, mapped, sdkerrors.IsRetryable(mapped))
		}
	}
}

func TestOrderUnavailablePreconditionClassification(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_MARKET_PRICE_UNAVAILABLE,
		orderv1.ErrorCode_ERROR_CODE_ORDER_LIMITS_UNAVAILABLE,
	} {
		err := connect.NewError(connect.CodeFailedPrecondition, errors.New("not ready"))
		detail, detailErr := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: code})
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		err.AddDetail(detail)
		mapped := MapConnectError(err)
		var api *sdkerrors.APIError
		if !errors.As(mapped, &api) || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s mapped=%T retryable=%v", code, mapped, sdkerrors.IsRetryable(mapped))
		}
	}
}
