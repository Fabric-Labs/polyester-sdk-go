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
		TriggerId:       7,
		SymbolId:        2,
		Symbol:          "BTC-USD",
		TriggerType:     triggersv1.TriggerType_STOP_LOSS,
		Status:          triggersv1.TriggerStatus_STATUS_ARMED,
		Side:            orderv1.Side_SELL,
		QtyScaled:       1_000_000,
		Details: &triggersv1.Trigger_Stop{
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
}

func TestTriggersListFromProto(t *testing.T) {
	msg := &triggersv1.ListTriggersResponse{
		Triggers: []*triggersv1.Trigger{{TriggerId: 1, SymbolId: 1}},
	}
	result := decode.TriggersListFromProto(msg)
	if len(result.Triggers) != 1 || result.Total != 1 {
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
