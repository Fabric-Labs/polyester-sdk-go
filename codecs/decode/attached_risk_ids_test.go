package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestCreateAndModifyResultsCarryAttachedRiskTriggerIDs(t *testing.T) {
	tp, sl := uint64(41), uint64(42)
	created, err := decode.OrderMutationFromProto(&orderv1.CreateOrderResponse{
		OrderId:             7,
		TakeProfitTriggerId: &tp,
		StopLossTriggerId:   &sl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.TakeProfitTriggerID != codecs.FormatUint64ID(tp) || created.StopLossTriggerID != codecs.FormatUint64ID(sl) || created.TrailingStopTriggerID != "" {
		t.Fatalf("created=%+v", created)
	}
	trailing := uint64(43)
	modified, err := decode.ModifyOrderFromProto(&orderv1.ModifyOrderResponse{
		ActionTaken:           orderv1.ModifyActionTaken_AMENDED,
		OldOrderId:            7,
		FinalOrderId:          7,
		TrailingStopTriggerId: &trailing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if modified.TrailingStopTriggerID != codecs.FormatUint64ID(trailing) || modified.TakeProfitTriggerID != "" || modified.StopLossTriggerID != "" {
		t.Fatalf("modified=%+v", modified)
	}
}
