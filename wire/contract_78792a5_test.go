package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func mapOrderRefusal(t *testing.T, code orderv1.ErrorCode, connectCode connect.Code) error {
	t.Helper()
	connectErr := connect.NewError(connectCode, errors.New("refused"))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: code})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	return MapConnectError(connectErr)
}

// The API key's spot-trading refusal (78), a key with no policy (27) and a
// master-account key whose market list excludes the pair (29) are permission
// refusals exactly like the sub-account policy's (23, 20): a final
// *errors.AuthError with a known outcome, carrying its own code.
func TestSpotTradePermissionRefusalsKeepTheirCodes(t *testing.T) {
	codes := []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_API_KEY_SPOT_TRADE_DENY,
		orderv1.ErrorCode_ERROR_CODE_API_KEY_POLICY_REQUIRED,
		orderv1.ErrorCode_ERROR_CODE_API_KEY_MARKET_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_SPOT_TRADE_DENY,
		orderv1.ErrorCode_ERROR_CODE_POLICY_MARKET_DENY,
	}
	if orderv1.ErrorCode_ERROR_CODE_API_KEY_SPOT_TRADE_DENY != 78 {
		t.Fatalf("API_KEY_SPOT_TRADE_DENY = %d, want 78", orderv1.ErrorCode_ERROR_CODE_API_KEY_SPOT_TRADE_DENY)
	}
	for _, code := range codes {
		t.Run(code.String(), func(t *testing.T) {
			mapped := mapOrderRefusal(t, code, connect.CodePermissionDenied)
			var authErr *sdkerrors.AuthError
			if !errors.As(mapped, &authErr) || authErr.Code != code.String() {
				t.Fatalf("mapped=%T %#v", mapped, mapped)
			}
			if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("%s must be final with a known outcome", code)
			}
		})
	}
}

// Iceberg and peg refusals, and the renamed quote-size refusal, keep their
// contract codes and are final.
func TestIcebergPegAndQuoteSizeRefusals(t *testing.T) {
	for _, tc := range []struct {
		code       orderv1.ErrorCode
		name       string
		connectErr connect.Code
	}{
		{orderv1.ErrorCode_ERROR_CODE_ICEBERG_NOT_ALLOWED, "ERROR_CODE_ICEBERG_NOT_ALLOWED", connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_ICEBERG_DISPLAY_TOO_SMALL, "ERROR_CODE_ICEBERG_DISPLAY_TOO_SMALL", connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_ICEBERG_DISPLAY_INVALID, "ERROR_CODE_ICEBERG_DISPLAY_INVALID", connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_PEG_LIMIT_VIOLATION, "ERROR_CODE_PEG_LIMIT_VIOLATION", connect.CodeFailedPrecondition},
		{orderv1.ErrorCode_ERROR_CODE_PEG_REFERENCE_UNAVAILABLE, "ERROR_CODE_PEG_REFERENCE_UNAVAILABLE", connect.CodeFailedPrecondition},
		{orderv1.ErrorCode_ERROR_CODE_INVALID_PEG, "ERROR_CODE_INVALID_PEG", connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_QUOTE_SIZE_TOO_SMALL, "ERROR_CODE_QUOTE_SIZE_TOO_SMALL", connect.CodeInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := mapOrderRefusal(t, tc.code, tc.connectErr)
			apiErr, ok := mapped.(*sdkerrors.APIError)
			if !ok || apiErr.Code != tc.name {
				t.Fatalf("mapped=%T %#v", mapped, mapped)
			}
			if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("%s must be final with a known outcome", tc.name)
			}
		})
	}
}

// An ended trigger answers its cancel, pause, resume or modify with
// TRIGGER_STATUS_INVALID: IsTriggerAlreadyEnded reports it, as "already
// done". TRIGGER_NOT_FOUND also names an ID of none of the caller's
// triggers, so it is not reported.
func TestIsTriggerAlreadyEnded(t *testing.T) {
	ended := mapOrderRefusal(t, orderv1.ErrorCode_ERROR_CODE_TRIGGER_STATUS_INVALID, connect.CodeFailedPrecondition)
	if !sdkerrors.IsTriggerAlreadyEnded(ended) || sdkerrors.IsRetryable(ended) || sdkerrors.MutationOutcomeUnknown(ended) {
		t.Fatalf("ended=%T %#v", ended, ended)
	}
	notFound := mapOrderRefusal(t, orderv1.ErrorCode_ERROR_CODE_TRIGGER_NOT_FOUND, connect.CodeNotFound)
	if sdkerrors.IsTriggerAlreadyEnded(notFound) {
		t.Fatalf("TRIGGER_NOT_FOUND must not read as ended: %#v", notFound)
	}
	if sdkerrors.IsTriggerAlreadyEnded(nil) || sdkerrors.IsTriggerAlreadyEnded(errors.New("x")) {
		t.Fatal("plain errors must not read as ended")
	}
}

// A refusal caused by an attached leg's exit names that leg: the violation's
// field path is the leg's attached-risk field and its rule is the code.
func TestAttachedLegMinimumRefusalNamesTheLeg(t *testing.T) {
	connectErr := connect.NewError(connect.CodeInvalidArgument, errors.New("Its stop-loss can't be placed when it triggers (MIN_NOTIONAL), so the order was not placed. Nothing changed."))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_MIN_NOTIONAL,
		Violations: []*orderv1.FieldViolation{{
			FieldPath: "order.attached_risk.stop_loss", RuleId: "MIN_NOTIONAL",
			Message: "Its stop-loss can't be placed when it triggers (MIN_NOTIONAL), so the order was not placed. Nothing changed.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(mapped, &validationErr) || validationErr.Code != "ERROR_CODE_MIN_NOTIONAL" ||
		len(validationErr.Violations) != 1 || validationErr.Violations[0].FieldPath != "order.attached_risk.stop_loss" ||
		validationErr.Violations[0].RuleID != "MIN_NOTIONAL" {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
}

// A trailing stop that has started tracking refuses a new nonzero activation
// price (315): a final refusal that changed nothing, under its generated name.
func TestTriggerActivationPriceNotModifiableIsAFinalRefusal(t *testing.T) {
	if orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE != 315 {
		t.Fatalf("TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE = %d, want 315", orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE)
	}
	mapped := mapOrderRefusal(t, orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE, connect.CodeFailedPrecondition)
	apiErr, ok := mapped.(*sdkerrors.APIError)
	if !ok || apiErr.Code != "ERROR_CODE_TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE" {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) || sdkerrors.IsTriggerAlreadyEnded(mapped) {
		t.Fatal("315 must be a final refusal with a known outcome")
	}
}
