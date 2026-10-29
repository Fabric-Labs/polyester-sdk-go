package codecs_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A source read from an order or trigger is sent back unchanged on create
// and modify.
func TestTriggerPriceSourceRoundTripsFromReadsToWrites(t *testing.T) {
	marketChild := &orderv1.RiskExecution{
		Execution: &orderv1.RiskExecution_MarketIoc{MarketIoc: &orderv1.RiskMarketIoc{}},
	}
	read := decode.OrderFromProto(&orderv1.Order{
		OrderId:  1,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit: &orderv1.AttachedRiskTakeProfit{Policy: &orderv1.TakeProfitPolicy{
				TriggerPriceTicks:  72000000000000,
				Child:              marketChild,
				TriggerPriceSource: orderv1.TriggerPriceSource_INDEX_PRICE,
			}},
			StopLoss: &orderv1.AttachedRiskStopLoss{Policy: &orderv1.StopLossPolicy{
				TriggerPriceTicks:  58000000000000,
				Child:              marketChild,
				TriggerPriceSource: orderv1.TriggerPriceSource_LAST_PRICE,
			}},
			Oco: true,
		},
	}).AttachedRisk
	readTrailing := decode.OrderFromProto(&orderv1.Order{
		OrderId:  2,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TrailingStop: &orderv1.AttachedRiskTrailingStop{Policy: &orderv1.TrailingStopPolicy{
				TrailingDistance:   &orderv1.TrailingStopPolicy_TrailingDistanceBps{TrailingDistanceBps: 150},
				TriggerPriceSource: orderv1.TriggerPriceSource_INDEX_PRICE,
			}},
		},
	}).AttachedRisk
	if read == nil || read.TakeProfit == nil || read.StopLoss == nil || readTrailing == nil || readTrailing.TrailingStop == nil {
		t.Fatalf("read=%+v trailing=%+v", read, readTrailing)
	}

	legs := &models.AttachedRiskInput{
		TakeProfit: &models.RiskLegInput{
			TriggerPrice:       models.PriceFromTicksInt(read.TakeProfit.TriggerPrice.Ticks()),
			TriggerPriceSource: read.TakeProfit.TriggerPriceSource,
		},
		StopLoss: &models.RiskLegInput{
			TriggerPrice:       models.PriceFromTicksInt(read.StopLoss.TriggerPrice.Ticks()),
			TriggerPriceSource: read.StopLoss.TriggerPriceSource,
		},
		OCO: read.Oco,
	}
	distance := readTrailing.TrailingStop.DistanceBps
	trailing := &models.AttachedRiskInput{
		TrailingStop: &models.TrailingStopInput{
			DistanceBps:        &distance,
			TriggerPriceSource: readTrailing.TrailingStop.TriggerPriceSource,
		},
	}

	symbolID := uint32(1)
	price := models.PriceFromDecimal("60000")
	create := func(risk *models.AttachedRiskInput) *orderv1.RiskPolicy {
		t.Helper()
		req, err := codecs.CreateOrderToProto(models.CreateOrderRequest{
			SymbolID:     &symbolID,
			Side:         "buy",
			OrderType:    "limit",
			Qty:          models.QtyFromScaledInt(100000),
			Price:        &price,
			AttachedRisk: risk,
		}, 8, 6)
		if err != nil {
			t.Fatal(err)
		}
		return req.GetOrder().GetAttachedRisk()
	}
	modify := func(risk *models.AttachedRiskInput) *orderv1.RiskPolicy {
		t.Helper()
		req, err := codecs.ModifyOrderToProto("BTC-USDT", 1, models.OrderKeyByID("100"), nil, nil, nil, nil, risk, nil, nil, 8)
		if err != nil {
			t.Fatal(err)
		}
		return req.GetNewAttachedRisk()
	}
	for name, encode := range map[string]func(*models.AttachedRiskInput) *orderv1.RiskPolicy{"create": create, "modify": modify} {
		policy := encode(legs)
		if policy.GetTakeProfit().GetTriggerPriceSource() != orderv1.TriggerPriceSource_INDEX_PRICE ||
			policy.GetStopLoss().GetTriggerPriceSource() != orderv1.TriggerPriceSource_LAST_PRICE {
			t.Fatalf("%s legs=%v", name, policy)
		}
		if got := encode(trailing).GetTrailingStop().GetTriggerPriceSource(); got != orderv1.TriggerPriceSource_INDEX_PRICE {
			t.Fatalf("%s trailing source=%v", name, got)
		}
	}

	stopRead := decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 3,
		SymbolId:  1,
		RuntimeDetails: &triggersv1.Trigger_Stop{Stop: &triggersv1.StopDetails{
			TriggerPriceTicks:  58000000000000,
			TriggerPriceSource: orderv1.TriggerPriceSource_INDEX_PRICE,
		}},
	})
	trailingRead := decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 4,
		SymbolId:  1,
		RuntimeDetails: &triggersv1.Trigger_Trailing{Trailing: &triggersv1.TrailingDetails{
			TrailingDistanceBps: 100,
			TriggerPriceSource:  orderv1.TriggerPriceSource_INDEX_PRICE,
		}},
	})
	if stopRead.Details == nil || trailingRead.Details == nil {
		t.Fatalf("stop=%+v trailing=%+v", stopRead, trailingRead)
	}
	trigger := models.PriceFromTicksInt(stopRead.Details.TriggerPrice.Ticks())
	stop, err := codecs.CreateTriggerToProto(1, "BTC-USDT", "stop_loss", &trigger, "sell",
		models.QtyFromDecimal("0.01"), "market", nil, stopRead.Details.TriggerPriceSource, "", nil, nil, false, 8,
		codecs.CreateTriggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stop.GetTrigger().GetStopLoss().GetTriggerPriceSource(); got != orderv1.TriggerPriceSource_INDEX_PRICE {
		t.Fatalf("stop_loss source=%v", got)
	}
	trailingBps := trailingRead.Details.TrailingDistanceBps
	trailingTrigger, err := codecs.CreateTriggerToProto(1, "BTC-USDT", "trailing_stop", nil, "sell",
		models.QtyFromDecimal("0.01"), "market", nil, trailingRead.Details.TriggerPriceSource, "", nil, nil, false, 8,
		codecs.CreateTriggerOptions{TrailingDistanceBps: &trailingBps})
	if err != nil {
		t.Fatal(err)
	}
	if got := trailingTrigger.GetTrigger().GetTrailingStop().GetTriggerPriceSource(); got != orderv1.TriggerPriceSource_INDEX_PRICE {
		t.Fatalf("trailing_stop source=%v", got)
	}
	takeProfit, err := codecs.CreateTriggerToProto(1, "BTC-USDT", "take_profit", &trigger, "sell",
		models.QtyFromDecimal("0.01"), "market", nil, "last", "", nil, nil, false, 8, codecs.CreateTriggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := takeProfit.GetTrigger().GetTakeProfit().GetTriggerPriceSource(); got != orderv1.TriggerPriceSource_LAST_PRICE {
		t.Fatalf("take_profit source=%v", got)
	}
}

func TestTriggerPriceSourceEmptySendsNothingAndUnknownIsRefused(t *testing.T) {
	symbolID := uint32(1)
	price := models.PriceFromDecimal("60000")
	createLeg := func(source string) (*orderv1.RiskPolicy, error) {
		req, err := codecs.CreateOrderToProto(models.CreateOrderRequest{
			SymbolID:  &symbolID,
			Side:      "buy",
			OrderType: "limit",
			Qty:       models.QtyFromScaledInt(100000),
			Price:     &price,
			AttachedRisk: &models.AttachedRiskInput{
				StopLoss: &models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("58000"), TriggerPriceSource: source},
			},
		}, 8, 6)
		return req.GetOrder().GetAttachedRisk(), err
	}
	policy, err := createLeg("")
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.GetStopLoss().GetTriggerPriceSource(); got != orderv1.TriggerPriceSource_TRIGGER_PRICE_SOURCE_UNSPECIFIED {
		t.Fatalf("empty source sent %v", got)
	}

	var validationErr *sdkerrors.ValidationError
	if _, err := createLeg("last_trade"); !errors.As(err, &validationErr) {
		t.Fatalf("unknown leg source err=%T %v", err, err)
	}
	distance := int32(100)
	_, err = codecs.RiskPolicyToProto(&models.AttachedRiskInput{
		TrailingStop: &models.TrailingStopInput{DistanceBps: &distance, TriggerPriceSource: "spot"},
	}, "new_attached_risk", "BTC-USDT")
	if !errors.As(err, &validationErr) {
		t.Fatalf("unknown trailing source err=%T %v", err, err)
	}
	trigger := models.PriceFromDecimal("58000")
	_, err = codecs.CreateTriggerToProto(1, "BTC-USDT", "stop_loss", &trigger, "sell",
		models.QtyFromDecimal("0.01"), "market", nil, "spot", "", nil, nil, false, 8, codecs.CreateTriggerOptions{})
	if !errors.As(err, &validationErr) {
		t.Fatalf("unknown trigger source err=%T %v", err, err)
	}
}
