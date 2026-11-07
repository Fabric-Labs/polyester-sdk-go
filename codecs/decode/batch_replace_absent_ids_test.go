package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

// An admission item the server sent without a successor or predecessor ID
// keeps that ID absent (empty), as status reads and the Python SDK do.
func TestBatchReplaceAdmissionKeepsAbsentIDsAbsent(t *testing.T) {
	admitted := orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED
	rejected := orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED
	result, err := decode.BatchReplaceFromProto(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 42,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{
			{ItemIndex: 0, Status: admitted, OldOrderId: 11, ActionTaken: orderv1.ModifyActionTaken_AMENDED},
			{ItemIndex: 1, Status: admitted, OldOrderId: 12, ReplacementOrderId: 99, ActionTaken: orderv1.ModifyActionTaken_REPLACED},
			{ItemIndex: 2, Status: rejected, OldOrderId: 13, Code: "ERROR_CODE_ORDER_ALREADY_TERMINAL"},
			{ItemIndex: 3, Status: rejected, ClientOrderId: "missing", Code: "ERROR_CODE_ORDER_UNKNOWN"},
		},
		AcceptedCount: 2,
		RejectedCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ old, replacement string }{
		{codecs.FormatID(11), ""},
		{codecs.FormatID(12), codecs.FormatID(99)},
		{codecs.FormatID(13), ""},
		{"", ""},
	}
	for i, item := range result.Results {
		if item.OldOrderID != want[i].old || item.ReplacementOrderID != want[i].replacement {
			t.Fatalf("item %d: old=%q replacement=%q, want old=%q replacement=%q",
				i, item.OldOrderID, item.ReplacementOrderID, want[i].old, want[i].replacement)
		}
	}
}
