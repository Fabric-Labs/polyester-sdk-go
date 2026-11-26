package codecs

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A refusal of a whole batch replace request, the SDK's own check before
// sending included, names a batch item's field under its index (items[i]),
// as the contract's FieldViolation.field_path says and as the Rust SDK names
// it. A modify has no items and names the field alone.
func TestBatchReplaceRefusalNamesTheItemIndex(t *testing.T) {
	valid := models.BatchReplaceItem{Key: models.OrderKeyByID(FormatID(100)), NewPrice: ptr(models.PriceFromDecimal("185"))}
	belowActivation := &models.AttachedRiskInput{TrailingStop: &models.TrailingStopInput{
		DistanceBps:     ptr(int32(150)),
		ActivationPrice: ptr(models.PriceFromDecimal("190")),
		LimitPrice:      ptr(models.PriceFromDecimal("185.5")),
	}}
	bad := models.BatchReplaceItem{Key: models.OrderKeyByID(FormatID(101)), NewAttachedRisk: belowActivation}
	_, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{valid, bad}, 3, nil, ptr("batch-1"), 6)
	violations := errors.Violations(err)
	if len(violations) != 1 || violations[0].FieldPath != "items[1].new_attached_risk.trailing_stop.limit_price_ticks" ||
		violations[0].RuleID != "buy_at_least_activation" {
		t.Fatalf("batch replace violations=%#v (err %v)", violations, err)
	}
	if !strings.Contains(err.Error(), "items[1].new_attached_risk.trailing_stop.limit_price must be at least activation_price") {
		t.Fatalf("batch replace err=%v", err)
	}

	both := models.BatchReplaceItem{Key: models.OrderKeyByID(FormatID(101)), NewAttachedRisk: belowActivation, ClearAttachedRisk: true}
	_, err = BatchReplaceOrdersToProto([]models.BatchReplaceItem{valid, both}, 3, nil, ptr("batch-1"), 6)
	var validation *errors.ValidationError
	if !stderrors.As(err, &validation) || validation.Msg != "set items[1].new_attached_risk or clear_attached_risk, not both" {
		t.Fatalf("set and clear err=%v", err)
	}

	// A modify names the field alone.
	_, err = ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, nil, nil, nil, belowActivation, false, nil, nil, 6)
	if v := errors.Violations(err); len(v) != 1 || v[0].FieldPath != "new_attached_risk.trailing_stop.limit_price_ticks" {
		t.Fatalf("modify violations=%#v", v)
	}
}

// An empty new attached risk on a batch item is refused with its index and
// the same hint a modify gives: clearing risk is ClearAttachedRisk.
func TestBatchReplaceEmptyRiskKeepsTheClearHint(t *testing.T) {
	valid := models.BatchReplaceItem{Key: models.OrderKeyByID(FormatID(100)), NewPrice: ptr(models.PriceFromDecimal("185"))}
	empty := models.BatchReplaceItem{Key: models.OrderKeyByID(FormatID(101)), NewAttachedRisk: &models.AttachedRiskInput{}}
	_, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{valid, empty}, 3, nil, ptr("batch-1"), 6)
	var validation *errors.ValidationError
	const want = "items[1].new_attached_risk requires take_profit and/or a stop leg; to remove the order's attached risk, set ClearAttachedRisk instead"
	if !stderrors.As(err, &validation) || validation.Msg != want {
		t.Fatalf("empty risk err=%v", err)
	}
}
