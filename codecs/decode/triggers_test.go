package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

func TestTriggerFromProtoMapsStopPrice(t *testing.T) {
	msg := &triggersv1.Trigger{
		TriggerId: 7,
		SymbolId:  2,
		Symbol:    "BTC-USD",
		Status:    triggersv1.TriggerStatus_STATUS_ARMED,
		QtyScaled: 1_000_000,
		Configuration: &triggersv1.Trigger_StopLoss{
			StopLoss: &triggersv1.ConditionalTrigger{
				TriggerPriceTicks: 50_000_000_000,
				Side:              orderv1.Side_SELL,
				Child: &triggersv1.ConditionalChildExecution{
					Execution: &triggersv1.ConditionalChildExecution_MarketIoc{MarketIoc: &triggersv1.TriggerMarketIoc{}},
				},
			},
		},
		RuntimeDetails: &triggersv1.Trigger_Stop{
			Stop: &triggersv1.StopDetails{TriggerPriceTicks: 50_000_000_000},
		},
		ClientTriggerId: "ct-1",
	}
	trigger := decode.TriggerFromProto(msg)
	if trigger.TriggerID != codecs.FormatUint64ID(7) {
		t.Fatalf("trigger_id=%q", trigger.TriggerID)
	}
	if trigger.TriggerPrice.Ticks != 50_000_000_000 || trigger.ClientTriggerID != "ct-1" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if trigger.TriggerType != "stop_loss" || trigger.Status != "armed" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if trigger.Side != "sell" || trigger.OrderType != "market" {
		t.Fatalf("trigger side/type=%+v", trigger)
	}
	if trigger.Details == nil || trigger.Details.Case != "stop" {
		t.Fatalf("details=%+v", trigger.Details)
	}
}

func TestTriggerStatusFromLabel(t *testing.T) {
	st, err := decode.TriggerStatusFromLabel("armed")
	if err != nil || st != triggersv1.TriggerStatus_STATUS_ARMED {
		t.Fatalf("armed: %v %v", st, err)
	}
	if _, err := decode.TriggerStatusFromLabel("nope"); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestTriggersListFromProto(t *testing.T) {
	msg := &triggersv1.ListTriggersResponse{
		Triggers:      []*triggersv1.Trigger{{TriggerId: 1, SymbolId: 1}},
		NextPageToken: "trig-page-2",
	}
	result := decode.TriggersListFromProto(msg)
	if len(result.Triggers) != 1 || result.Total != 1 || result.NextPageToken != "trig-page-2" {
		t.Fatalf("result=%+v", result)
	}
}

func TestTriggerEventsListFromProto(t *testing.T) {
	msg := &triggersv1.ListTriggerEventsResponse{
		Events: []*triggersv1.TriggerEvent{{
			TriggerId: 1,
			EventType: triggersv1.TriggerEventType_EVENT_FIRED,
			TsNs:      123,
		}},
		NextPageToken: "evt-page-2",
	}
	result := decode.TriggerEventsListFromProto(msg)
	if len(result.Events) != 1 || result.NextPageToken != "evt-page-2" {
		t.Fatalf("result=%+v", result)
	}
}

func TestGetTriggerFromProto(t *testing.T) {
	msg := &triggersv1.GetTriggerResponse{
		Trigger: &triggersv1.Trigger{TriggerId: 3, SymbolId: 1},
	}
	trigger := decode.GetTriggerFromProto(msg)
	if trigger == nil || trigger.TriggerID != codecs.FormatUint64ID(3) {
		t.Fatalf("trigger=%+v", trigger)
	}
}
