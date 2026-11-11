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

// Modify takes 0 to 10000 bps, as ModifyTriggerRequest does: the server
// decides 10000 by the trigger's side and type (POLY-9426).
func TestTriggerModifyTakesSlippageUpTo10000Bps(t *testing.T) {
	over := int32(10_001)
	_, err := ModifyTriggerToProto("1", 7, "", nil, ModifyTriggerOptions{MaxSlippageBps: &over})
	wantValidation(t, err, "between 0 and 10000")
	for _, ok := range []int32{0, 9_999, 10_000} {
		req, err := ModifyTriggerToProto("1", 7, "", nil, ModifyTriggerOptions{MaxSlippageBps: &ok})
		if err != nil || req.GetMaxSlippageBps() != ok {
			t.Fatalf("modify %d bps: %v %+v", ok, err, req)
		}
	}
}
