package codecs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestModifyTriggerOmitsUnsetFieldsAndClearsWithZero(t *testing.T) {
	zeroPrice := models.PriceFromTicksInt(0)
	zeroBps := int32(0)
	req, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{
		ActivationPrice: &zeroPrice,
		MaxSlippageBps:  &zeroBps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.ActivationPriceTicks == nil || *req.ActivationPriceTicks != 0 {
		t.Fatalf("activation_price_ticks=%v", req.ActivationPriceTicks)
	}
	if req.GetMaxSlippageBps() != 0 || req.MaxSlippage == nil {
		t.Fatalf("max_slippage=%+v", req.MaxSlippage)
	}
	if req.TriggerPriceTicks != nil || req.LimitPriceTicks != nil {
		t.Fatalf("omitted fields must stay unset: %+v", req)
	}

	preserve, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{
		TriggerPrice: pricePtr(models.PriceFromDecimal("101")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if preserve.ActivationPriceTicks != nil || preserve.MaxSlippage != nil {
		t.Fatalf("omit must preserve: %+v", preserve)
	}
}

func TestTriggerSlippageBpsCap(t *testing.T) {
	distance := int32(100)
	ok := int32(10000)
	if _, err := CreateTriggerToProto(
		1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceBps: &distance, MaxSlippageBps: &ok},
	); err != nil {
		t.Fatalf("10000 bps should be accepted: %v", err)
	}
	tooHigh := int32(10001)
	_, err := CreateTriggerToProto(
		1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceBps: &distance, MaxSlippageBps: &tooHigh},
	)
	if err == nil {
		t.Fatal("expected create BPS cap rejection")
	}
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}

	_, err = ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageBps: &tooHigh})
	if err == nil {
		t.Fatal("expected modify BPS cap rejection")
	}
	zero := int32(0)
	if _, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageBps: &zero}); err != nil {
		t.Fatalf("modify zero must clear, not reject: %v", err)
	}
}

func pricePtr(p models.PriceInput) *models.PriceInput { return &p }
