package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/proto"
)

func TestServiceUnavailableClassification(t *testing.T) {
	cases := []struct {
		name   string
		detail proto.Message
	}{
		{"orders/RATE_LIMIT_POLICY_UNAVAILABLE", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_POLICY_UNAVAILABLE}},
		{"orders/POLICY_UNAVAILABLE", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_POLICY_UNAVAILABLE}},
		{"orders/FEE_SCHEDULE_UNAVAILABLE", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_FEE_SCHEDULE_UNAVAILABLE}},
		{"orders/UNAVAILABLE", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_UNAVAILABLE}},
		{"orders/PAIR_BACKLOGGED", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_PAIR_BACKLOGGED}},
		{"orders/ORDER_MODIFY_IN_PROGRESS", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_ORDER_MODIFY_IN_PROGRESS}},
		{"orders/ORDER_PLACEMENT_PENDING", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_ORDER_PLACEMENT_PENDING}},
		{"orders/SETTLEMENT_BACKLOGGED", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_SETTLEMENT_BACKLOGGED}},
		{"orders/TRIGGER_CONTROLS_BUSY", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_TRIGGER_CONTROLS_BUSY}},
		{"orders/TRIGGER_CONTROL_IN_PROGRESS", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_TRIGGER_CONTROL_IN_PROGRESS}},
		{"orders/ACCOUNT_ROUTING_INVALID", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_ACCOUNT_ROUTING_INVALID}},
		{"orders/UPSTREAM_ERROR", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_UPSTREAM_ERROR}},
		{"withdraw/SERVICE_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE}},
		{"withdraw/ACCOUNT_SHARD_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE}},
		{"withdraw/CAPITAL_VIEW_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_CAPITAL_VIEW_UNAVAILABLE}},
		{"withdraw/CHAIN_METADATA_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_CHAIN_METADATA_UNAVAILABLE}},
		{"withdraw/DESTINATION_VALIDATION_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_DESTINATION_VALIDATION_UNAVAILABLE}},
		{"withdraw/FEE_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_FEE_UNAVAILABLE}},
		{"withdraw/STEP_UP_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_STEP_UP_UNAVAILABLE}},
		{"withdraw/SUPPLY_UNAVAILABLE", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_SUPPLY_UNAVAILABLE}},
		{"transfer/SERVICE_UNAVAILABLE", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE}},
		{"transfer/ACCOUNT_SHARD_UNAVAILABLE", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE}},
		{"transfer/CAPITAL_VIEW_UNAVAILABLE", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_CAPITAL_VIEW_UNAVAILABLE}},
		{"transfer/STEP_UP_UNAVAILABLE", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_STEP_UP_UNAVAILABLE}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := connect.NewError(connect.CodeUnavailable, errors.New("retry later"))
			detail, detailErr := connect.NewErrorDetail(tc.detail)
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
			code := tc.detail.ProtoReflect().Descriptor().Fields().ByName("code")
			wantCode := string(code.Enum().Values().ByNumber(tc.detail.ProtoReflect().Get(code).Enum()).Name())
			if server.Code != wantCode || server.Status != 503 {
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

func TestFundMovementUnavailablePreconditionClassification(t *testing.T) {
	for _, message := range []proto.Message{
		&transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_SMART_ACCOUNT_UNAVAILABLE},
		&withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_SOURCE_SMART_ACCOUNT_UNAVAILABLE},
	} {
		err := connect.NewError(connect.CodeFailedPrecondition, errors.New("not ready"))
		detail, detailErr := connect.NewErrorDetail(message)
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		err.AddDetail(detail)
		mapped := MapConnectError(err)
		var api *sdkerrors.APIError
		if !errors.As(mapped, &api) || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%T mapped=%T retryable=%v", message, mapped, sdkerrors.IsRetryable(mapped))
		}
	}
}
