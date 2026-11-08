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
}
