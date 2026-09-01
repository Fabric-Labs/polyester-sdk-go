package codecs

import (
	"errors"
	"fmt"
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

func TestTrailingDistanceBpsRange(t *testing.T) {
	for _, value := range []int32{0, 10_001, -1} {
		t.Run(fmt.Sprintf("create_%d", value), func(t *testing.T) {
			_, err := CreateTriggerToProto(
				1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
				"market", nil, "", "", nil, nil, false, 8,
				CreateTriggerOptions{TrailingDistanceBps: &value},
			)
			var validationErr *sdkerrors.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("trailing_distance_bps=%d: want ValidationError, got %T: %v", value, err, err)
			}
		})
	}
	for _, value := range []int32{-1, 0, 10_001} {
		t.Run(fmt.Sprintf("modify_%d", value), func(t *testing.T) {
			_, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{TrailingDistanceBps: &value})
			var validationErr *sdkerrors.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("trailing_distance_bps=%d: want ValidationError, got %T: %v", value, err, err)
			}
		})
	}
	for _, value := range []int32{1, 10_000} {
		t.Run(fmt.Sprintf("boundary_%d", value), func(t *testing.T) {
			if _, err := CreateTriggerToProto(
				1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
				"market", nil, "", "", nil, nil, false, 8,
				CreateTriggerOptions{TrailingDistanceBps: &value},
			); err != nil {
				t.Fatalf("create trailing_distance_bps=%d rejected: %v", value, err)
			}
			if _, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{TrailingDistanceBps: &value}); err != nil {
				t.Fatalf("modify trailing_distance_bps=%d rejected: %v", value, err)
			}
		})
	}
}

func TestLadderRequiresPriceMinStrictlyBelowMax(t *testing.T) {
	for _, tc := range []struct {
		name string
		min  string
		max  string
	}{
		{name: "equal", min: "100", max: "100"},
		{name: "inverted", min: "101", max: "100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			minPrice := models.PriceFromDecimal(tc.min)
			maxPrice := models.PriceFromDecimal(tc.max)
			_, err := CreateTriggerToProto(
				1, "BTC-USDT", "ladder", nil, "buy", models.QtyFromDecimal("0.1"),
				"limit", nil, "", "", nil, nil, false, 8,
				CreateTriggerOptions{LadderPriceMin: &minPrice, LadderPriceMax: &maxPrice},
			)
			var validationErr *sdkerrors.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("min=%s max=%s: want ValidationError, got %T: %v", tc.min, tc.max, err, err)
			}
		})
	}
}

func pricePtr(p models.PriceInput) *models.PriceInput { return &p }
