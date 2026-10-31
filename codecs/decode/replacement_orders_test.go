package decode

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

func TestOrderFromProtoExposesInheritedCumQty(t *testing.T) {
	order := OrderFromProto(&orderv1.Order{
		OrderId: 2, SymbolId: 1, CumQtyScaled: 7, InheritedCumQtyScaled: 5,
	})
	if order.InheritedCumQty.Scaled() != 5 || order.CumQty.Scaled() != 7 {
		t.Fatalf("inherited=%d cum=%d", order.InheritedCumQty.Scaled(), order.CumQty.Scaled())
	}
	if id := order.InheritedCumQty.SymbolID(); id == nil || *id != 1 {
		t.Fatalf("inherited quantity must carry the order's symbol: %v", id)
	}
	if original := OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 1}); original.InheritedCumQty.Scaled() != 0 {
		t.Fatalf("original order inherited=%d", original.InheritedCumQty.Scaled())
	}
}

func TestBatchReplaceItemsExposeActionTaken(t *testing.T) {
	admitted := orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED
	rejected := orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED
	receipt, err := BatchReplaceFromProto(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 9,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{
			{ItemIndex: 0, Status: admitted, OldOrderId: 1, ReplacementOrderId: 2, ActionTaken: orderv1.ModifyActionTaken_REPLACED},
			{ItemIndex: 1, Status: admitted, OldOrderId: 3, ActionTaken: orderv1.ModifyActionTaken_AMENDED},
			{ItemIndex: 2, Status: rejected, OldOrderId: 4, Code: "ERROR_CODE_MAX_QTY"},
		},
		AcceptedCount: 2,
		RejectedCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"replaced", "amended", ""} {
		if got := receipt.Results[i].ActionTaken; got != want {
			t.Fatalf("admission item %d action=%q want %q", i, got, want)
		}
	}

	status, err := BatchReplaceStatusFromProto(&orderv1.GetBatchReplaceStatusResponse{
		BatchRequestId:  9,
		AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED,
		Items: []*orderv1.BatchReplaceStatusItem{
			{ItemIndex: 0, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING, OldOrderId: 1, ReplacementOrderId: 2, ActionTaken: orderv1.ModifyActionTaken_REPLACED},
			{ItemIndex: 1, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_TERMINAL, OldOrderId: 3, ActionTaken: orderv1.ModifyActionTaken_AMENDED},
		},
		AcceptedCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Items[0].ActionTaken != "replaced" || status.Items[1].ActionTaken != "amended" {
		t.Fatalf("status items=%+v", status.Items)
	}
}

func TestBatchCreateRejectionExposesExistingOrderID(t *testing.T) {
	result, err := BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
		Results: []*orderv1.BatchCreateResultItem{
			{ClientOrderId: "dup", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{
				Error: &orderv1.ErrorDetail{
					Code:            orderv1.ErrorCode_ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID,
					ExistingOrderId: proto.Uint64(77),
				},
			}}},
			{ClientOrderId: "dup-in-batch", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{
				Error: &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID},
			}}},
		},
		RejectedCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Results[0].ExistingOrderID; got != codecs.FormatUint64ID(77) {
		t.Fatalf("existing order id=%q", got)
	}
	if got := result.Results[1].ExistingOrderID; got != "" {
		t.Fatalf("unknown holder must stay empty, got %q", got)
	}
}
