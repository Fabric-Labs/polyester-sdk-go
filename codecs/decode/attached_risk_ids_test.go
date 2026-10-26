package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func idPtr(v uint64) *uint64 { return &v }

func TestCreateAndModifyResultsCarryAttachedRiskTriggerIDs(t *testing.T) {
	all := models.AttachedRiskTriggerIDs{
		TakeProfitTriggerID:   codecs.FormatUint64ID(41),
		StopLossTriggerID:     codecs.FormatUint64ID(42),
		TrailingStopTriggerID: codecs.FormatUint64ID(43),
	}
	created, err := decode.OrderMutationFromProto(&orderv1.CreateOrderResponse{
		OrderId:               7,
		TakeProfitTriggerId:   idPtr(41),
		StopLossTriggerId:     idPtr(42),
		TrailingStopTriggerId: idPtr(43),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.AttachedRiskTriggerIDs != all {
		t.Fatalf("create trigger IDs=%+v want %+v", created.AttachedRiskTriggerIDs, all)
	}
	modified, err := decode.ModifyOrderFromProto(&orderv1.ModifyOrderResponse{
		ActionTaken:           orderv1.ModifyActionTaken_AMENDED,
		OldOrderId:            7,
		FinalOrderId:          7,
		TakeProfitTriggerId:   idPtr(41),
		StopLossTriggerId:     idPtr(42),
		TrailingStopTriggerId: idPtr(43),
	})
	if err != nil {
		t.Fatal(err)
	}
	if modified.AttachedRiskTriggerIDs != all {
		t.Fatalf("modify trigger IDs=%+v want %+v", modified.AttachedRiskTriggerIDs, all)
	}
}

func TestResultsOmitUnconfiguredLegTriggerIDs(t *testing.T) {
	created, err := decode.OrderMutationFromProto(&orderv1.CreateOrderResponse{OrderId: 7, StopLossTriggerId: idPtr(0)})
	if err != nil {
		t.Fatal(err)
	}
	if created.AttachedRiskTriggerIDs != (models.AttachedRiskTriggerIDs{}) {
		t.Fatalf("create trigger IDs=%+v want none", created.AttachedRiskTriggerIDs)
	}
	modified, err := decode.ModifyOrderFromProto(&orderv1.ModifyOrderResponse{
		ActionTaken: orderv1.ModifyActionTaken_REPLACED, OldOrderId: 7, FinalOrderId: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if modified.AttachedRiskTriggerIDs != (models.AttachedRiskTriggerIDs{}) {
		t.Fatalf("modify trigger IDs=%+v want none", modified.AttachedRiskTriggerIDs)
	}
}
