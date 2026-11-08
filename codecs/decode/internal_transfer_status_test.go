package decode_test

import (
	"reflect"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func oneTokenReceipt(status transferv1.InternalTransferStatus) *transferv1.CreateInternalTransferResponse {
	return &transferv1.CreateInternalTransferResponse{
		RequestId:  "req",
		TransferId: "xfer",
		AssetId:    7,
		AssetCode:  "USDT",
		AmountE18:  &typev1.U128{Lo: 1_000_000_000_000_000_000},
		Status:     status,
	}
}

// POLY-8824: receipts that differ only in their status decode to different
// results.
func TestPoly8824InternalTransferReceiptKeepsItsStatus(t *testing.T) {
	decoded := map[transferv1.InternalTransferStatus]models.InternalTransferResult{}
	for _, status := range []transferv1.InternalTransferStatus{
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_ACCEPTED,
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_REJECTED,
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_FAILED,
	} {
		result, err := decode.InternalTransferFromProto(oneTokenReceipt(status))
		if err != nil {
			t.Fatal(err)
		}
		for other, seen := range decoded {
			if reflect.DeepEqual(seen, result) {
				t.Fatalf("%v and %v decode to the same result %+v", other, status, result)
			}
		}
		decoded[status] = result
	}
	for status, want := range map[transferv1.InternalTransferStatus]string{
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_ACCEPTED:    "accepted",
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_REJECTED:    "rejected",
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_FAILED:      "failed",
		transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_UNSPECIFIED: "",
		transferv1.InternalTransferStatus(42):                                  "UNKNOWN(42)",
	} {
		result, err := decode.InternalTransferFromProto(oneTokenReceipt(status))
		if err != nil || result.Status != want {
			t.Fatalf("%v: status %q err %v, want %q", status, result.Status, err, want)
		}
		qty, qtyErr := result.Quantity.Int64()
		if qtyErr == nil && qty != 1_000_000_000_000_000_000 {
			t.Fatalf("%v: quantity %d", status, qty)
		}
	}
	// A receipt without IDs is still a contract violation, whatever its status.
	missing := oneTokenReceipt(transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_REJECTED)
	missing.TransferId = ""
	if _, err := decode.InternalTransferFromProto(missing); err == nil {
		t.Fatal("missing transfer_id accepted")
	}
}
