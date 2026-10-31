package decode

import (
	"testing"

	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"google.golang.org/protobuf/proto"
)

func TestTrailingDetailsExposeCurrentThresholdAsTriggerPrice(t *testing.T) {
	armed := TriggerFromProto(&triggersv1.Trigger{TriggerId: 1, SymbolId: 1, RuntimeDetails: &triggersv1.Trigger_Trailing{
		Trailing: &triggersv1.TrailingDetails{PeakPriceTicks: 110_000_000_000, TriggerPriceTicks: proto.Int64(105_000_000_000)},
	}})
	if armed.Details == nil || armed.Details.TriggerPrice.Ticks() != 105_000_000_000 {
		t.Fatalf("details=%+v", armed.Details)
	}
	unarmed := TriggerFromProto(&triggersv1.Trigger{TriggerId: 1, SymbolId: 1, RuntimeDetails: &triggersv1.Trigger_Trailing{
		Trailing: &triggersv1.TrailingDetails{},
	}})
	if unarmed.Details == nil || unarmed.Details.TriggerPrice.Ticks() != 0 {
		t.Fatalf("unarmed details=%+v", unarmed.Details)
	}
}
