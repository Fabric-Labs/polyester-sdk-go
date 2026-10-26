package codecs

import (
	"strings"
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func ptr[T any](v T) *T { return &v }

func attachedOCO(tp, sl string) *models.AttachedRiskInput {
	return &models.AttachedRiskInput{
		TakeProfit: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal(tp)},
		StopLoss:   &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal(sl)},
		OCO:        true,
	}
}

func TestCreateOrderEncodesAttachedOCO(t *testing.T) {
	symbol := "BTC-USDT"
	symbolID := uint32(1)
	tif := "gtc"
	price := models.PriceFromDecimal("60000")
	risk := attachedOCO("72000.5", "58000")
	risk.TakeProfit.LimitPrice = ptr(models.PriceFromDecimal("71999.123456789"))
	req, err := CreateOrderToProto(models.CreateOrderRequest{
		Symbol:       &symbol,
		SymbolID:     &symbolID,
		Side:         "buy",
		OrderType:    "limit",
		TIF:          &tif,
		Qty:          models.QtyFromScaledInt(100000),
		Price:        &price,
		AttachedRisk: risk,
	}, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	got := req.GetOrder().GetAttachedRisk()
	if got == nil || !got.GetOco() {
		t.Fatalf("attached_risk=%v", got)
	}
	tp := got.GetTakeProfit()
	if tp.GetTriggerPriceTicks() != 72000500000000 {
		t.Fatalf("take_profit trigger=%d", tp.GetTriggerPriceTicks())
	}
	if tp.GetChild().GetLimitGtc().GetPriceTicks() != 71999123456789 {
		t.Fatalf("take_profit child=%v", tp.GetChild())
	}
	sl := got.GetStopLoss()
	if sl.GetTriggerPriceTicks() != 58000000000000 {
		t.Fatalf("stop_loss trigger=%d", sl.GetTriggerPriceTicks())
	}
	if sl.GetChild().GetMarketIoc() == nil {
		t.Fatalf("stop_loss child=%v", sl.GetChild())
	}
	if got.GetTrailingStop() != nil {
		t.Fatal("unexpected trailing stop")
	}
}

func TestCreateOrderEncodesAttachedTrailingStop(t *testing.T) {
	symbolID := uint32(2)
	price := models.PriceFromDecimal("2420")
	req, err := CreateOrderToProto(models.CreateOrderRequest{
		SymbolID:  &symbolID,
		Side:      "buy",
		OrderType: "limit",
		Qty:       models.QtyFromScaledInt(2315),
		Price:     &price,
		AttachedRisk: &models.AttachedRiskInput{
			TakeProfit: &models.RiskLegInput{TriggerPrice: models.PriceFromTicksInt(2600000000000)},
			TrailingStop: &models.TrailingStopInput{
				DistanceBps:     ptr(int32(150)),
				MaxSlippageBps:  ptr(int32(50)),
				ActivationPrice: ptr(models.PriceFromDecimal("2500")),
			},
			OCO: true,
		},
	}, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	trailing := req.GetOrder().GetAttachedRisk().GetTrailingStop()
	if trailing.GetTrailingDistanceBps() != 150 || trailing.GetMaxSlippageBps() != 50 {
		t.Fatalf("trailing=%v", trailing)
	}
	if trailing.GetActivationPriceTicks() != 2500000000000 {
		t.Fatalf("activation=%d", trailing.GetActivationPriceTicks())
	}
	if req.GetOrder().GetAttachedRisk().GetTakeProfit().GetTriggerPriceTicks() != 2600000000000 {
		t.Fatal("take_profit ticks were not passed through")
	}
}

func TestRiskPolicyRejectsInvalidShapes(t *testing.T) {
	cases := map[string]struct {
		in   *models.AttachedRiskInput
		want string
	}{
		"empty": {&models.AttachedRiskInput{}, "requires take_profit and/or a stop leg"},
		"oco without stop": {&models.AttachedRiskInput{
			TakeProfit: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("1")},
			OCO:        true,
		}, "oco requires take_profit and exactly one stop leg"},
		"oco without take profit": {&models.AttachedRiskInput{
			StopLoss: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("1")},
			OCO:      true,
		}, "oco requires take_profit and exactly one stop leg"},
		"two stop legs": {&models.AttachedRiskInput{
			StopLoss:     &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("1")},
			TrailingStop: &models.TrailingStopInput{DistanceBps: ptr(int32(10))},
		}, "at most one of stop_loss or trailing_stop"},
		"missing trigger": {&models.AttachedRiskInput{
			StopLoss: &models.RiskLegInput{},
		}, "attached_risk.stop_loss.trigger_price is required"},
		"zero trigger": {&models.AttachedRiskInput{
			StopLoss: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("0")},
		}, "attached_risk.stop_loss.trigger_price must be positive"},
		"trailing without distance": {&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{},
		}, "exactly one of distance_ticks or distance_bps"},
		"trailing with both distances": {&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{DistanceTicks: ptr(int64(1)), DistanceBps: ptr(int32(1))},
		}, "exactly one of distance_ticks or distance_bps"},
		"trailing with both slippages": {&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{DistanceBps: ptr(int32(1)), MaxSlippageTicks: ptr(int64(1)), MaxSlippageBps: ptr(int32(1))},
		}, "at most one of max_slippage_ticks or max_slippage_bps"},
		"trailing bps out of range": {&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{DistanceBps: ptr(int32(10001))},
		}, "distance_bps must be between 1 and 10000"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := RiskPolicyToProto(tc.in, "attached_risk", "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestModifyEncodesAttachedRiskOnlyPatch(t *testing.T) {
	req, err := ModifyOrderToProto(
		"SOL-USDT", 3, models.OrderKeyByID("100"), nil, ptr("mod-1"),
		nil, nil, attachedOCO("185.25", "140.5"), nil, nil, 6,
	)
	if err != nil {
		t.Fatal(err)
	}
	if req.NewPriceTicks != nil || req.NewQtyScaled != nil {
		t.Fatalf("unexpected price/qty patch: %v", req)
	}
	risk := req.GetNewAttachedRisk()
	if risk.GetTakeProfit().GetTriggerPriceTicks() != 185250000000 || risk.GetStopLoss().GetTriggerPriceTicks() != 140500000000 || !risk.GetOco() {
		t.Fatalf("new_attached_risk=%v", risk)
	}
	if _, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID("100"), nil, nil, nil, nil, nil, nil, nil, 6); err == nil ||
		!strings.Contains(err.Error(), "new_attached_risk") {
		t.Fatalf("empty patch err=%v", err)
	}
	_, err = ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID("100"), nil, nil, nil, nil,
		&models.AttachedRiskInput{OCO: true}, nil, nil, 6)
	if err == nil || !strings.Contains(err.Error(), "new_attached_risk requires take_profit") {
		t.Fatalf("invalid patch err=%v", err)
	}
}

func TestBatchReplaceEncodesAttachedRiskOnlyItem(t *testing.T) {
	req, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{{
		Key:             models.OrderKeyByClientID("cid-1"),
		NewAttachedRisk: &models.AttachedRiskInput{StopLoss: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("83589")}},
	}}, 1, nil, ptr("batch-1"), 8)
	if err != nil {
		t.Fatal(err)
	}
	item := req.GetItems()[0]
	if item.GetNewAttachedRisk().GetStopLoss().GetTriggerPriceTicks() != 83589000000000 {
		t.Fatalf("item=%v", item)
	}
	if _, ok := item.GetNewAttachedRisk().GetStopLeg().(*orderv1.RiskPolicy_StopLoss); !ok {
		t.Fatalf("stop leg=%T", item.GetNewAttachedRisk().GetStopLeg())
	}
}
