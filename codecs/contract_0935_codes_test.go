package codecs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// An attached exit's refused values carry the codes the Orders API answers
// for them on every transport: a trailing stop's the codes a standalone
// trailing stop answers, a take-profit's or stop-loss's the trigger and
// limit price codes.
func TestAttachedRiskRefusalsCarryTheAPIsCodes(t *testing.T) {
	i32 := func(v int32) *int32 { return &v }
	i64 := func(v int64) *int64 { return &v }
	price := func(v string) *models.PriceInput { p := models.PriceFromDecimal(v); return &p }
	trailing := func(edit func(*models.TrailingStopInput)) *models.AttachedRiskInput {
		in := models.TrailingStopInput{DistanceBps: i32(100)}
		edit(&in)
		return &models.AttachedRiskInput{TrailingStop: &in}
	}
	for _, tc := range []struct {
		name string
		in   *models.AttachedRiskInput
		want orderv1.ErrorCode
	}{
		{"trailing distance ticks zero", trailing(func(in *models.TrailingStopInput) { in.DistanceBps, in.DistanceTicks = nil, i64(0) }), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"trailing distance bps zero", trailing(func(in *models.TrailingStopInput) { in.DistanceBps = i32(0) }), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"trailing distance bps over 10000", trailing(func(in *models.TrailingStopInput) { in.DistanceBps = i32(10001) }), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"slippage ticks zero", trailing(func(in *models.TrailingStopInput) { in.MaxSlippageTicks = i64(0) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID},
		{"slippage ticks negative", trailing(func(in *models.TrailingStopInput) { in.MaxSlippageTicks = i64(-1) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID},
		{"slippage bps zero", trailing(func(in *models.TrailingStopInput) { in.MaxSlippageBps = i32(0) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID},
		{"slippage bps of 10000", trailing(func(in *models.TrailingStopInput) { in.MaxSlippageBps = i32(10000) }), orderv1.ErrorCode_ERROR_CODE_MAX_SLIPPAGE_INVALID},
		{"activation price negative", trailing(func(in *models.TrailingStopInput) { in.ActivationPrice = price("-1") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_INVALID},
		{"take-profit trigger price zero", &models.AttachedRiskInput{TakeProfit: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("0")}}, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"stop-loss limit price zero", &models.AttachedRiskInput{StopLoss: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("1"), LimitPrice: price("0")}}, orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
	} {
		_, err := RiskPolicyToProto(tc.in, "attached_risk", "BTC-USDT")
		requireCode(t, tc.name, err, tc.want)
	}
	// Values within the ranges are encoded.
	in := trailing(func(in *models.TrailingStopInput) {
		in.MaxSlippageBps = i32(9999)
		in.ActivationPrice = price("1")
	})
	if _, err := RiskPolicyToProto(in, "attached_risk", "BTC-USDT"); err != nil {
		t.Fatal(err)
	}
}

// A trigger modify that names no field to change is refused as the API
// refuses it: TRIGGER_PATCH_EMPTY, one violation with an empty field path and
// rule patch_empty. A symbol alone is no field to change.
func TestEmptyTriggerModifyIsTriggerPatchEmpty(t *testing.T) {
	_, err := ModifyTriggerToProto(FormatID(1234567890), 1, "BTC-USDT", nil, ModifyTriggerOptions{})
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Code != orderv1.ErrorCode_ERROR_CODE_TRIGGER_PATCH_EMPTY.String() {
		t.Fatalf("err = %T %+v", err, err)
	}
	if len(validation.Violations) != 1 || validation.Violations[0].FieldPath != "" || validation.Violations[0].RuleID != "patch_empty" {
		t.Fatalf("violations = %+v", validation.Violations)
	}
}

// A client list ID is checked as the contract does, everywhere it is read,
// and refused with CLIENT_LIST_ID_INVALID.
func TestClientListIDRefusalsCarryTheirCode(t *testing.T) {
	for _, id := range []string{"", " oco-1", "oco+1", "0123456789012345678901234567890123456"} {
		_, err := ValidateClientListID(id)
		requireCode(t, "client list ID "+id, err, orderv1.ErrorCode_ERROR_CODE_CLIENT_LIST_ID_INVALID)
	}
	for _, id := range []string{"a", "oco-1.A_b:c/d", "012345678901234567890123456789012345"} {
		if got, err := ValidateClientListID(id); err != nil || got != id {
			t.Fatalf("ValidateClientListID(%q) = %q, %v", id, got, err)
		}
	}
}
