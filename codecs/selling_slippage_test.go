package codecs

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A 100% (10000 bps) cap leaves anything that sells no positive price, so the
// backend rejects it with MAX_SLIPPAGE_INVALID.

func TestSellMarketOrderCapsSlippageAt9999Bps(t *testing.T) {
	sell := marketCreateRequest(t)
	sell.Side = "sell"
	sell.MaxSlippageBps = int32Ptr(10_000)
	_, err := CreateOrderToProto(sell, 8, 0)
	wantValidation(t, err, "between 1 and 9999")

	sell.MaxSlippageBps = int32Ptr(9_999)
	if _, err := CreateOrderToProto(sell, 8, 0); err != nil {
		t.Fatalf("sell market accepts 9999 bps: %v", err)
	}
	buy := marketCreateRequest(t)
	buy.MaxSlippageBps = int32Ptr(10_000)
	if _, err := CreateOrderToProto(buy, 8, 0); err != nil {
		t.Fatalf("buy market accepts 10000 bps: %v", err)
	}
}

func TestAttachedTrailingStopCapsSlippageAt9999BpsOnEitherSide(t *testing.T) {
	for _, bps := range []int32{9_999, 10_000} {
		_, err := RiskPolicyToProto(&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{DistanceBps: int32Ptr(100), MaxSlippageBps: int32Ptr(bps)},
		}, "attached_risk", "")
		if bps == 9_999 {
			if err != nil {
				t.Fatalf("9999 bps rejected: %v", err)
			}
			continue
		}
		wantValidation(t, err, "attached_risk.trailing_stop.max_slippage_bps must be between 1 and 9999")
	}
}

func TestTriggerModifyCapsSlippageAt9999Bps(t *testing.T) {
	full := int32(10_000)
	_, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageBps: &full})
	wantValidation(t, err, "between 1 and 9999")
	ok := int32(9_999)
	if _, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageBps: &ok}); err != nil {
		t.Fatalf("modify accepts 9999 bps: %v", err)
	}
}
