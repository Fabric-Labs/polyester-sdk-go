package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

func TestTransferRowFromProto(t *testing.T) {
	msg := &ledgerrdv1.TransferRow{
		AssetId:      2,
		AmountE18:    &typev1.U128{Lo: 1000},
		TransferCode: 5,
		AccountCode:  1,
		TsUs:         999,
		IsDebit:      true,
		FlowId:       "flow-abc",
	}
	transfer := decode.TransferRowFromProto(msg)
	if transfer.Amount != "1000" || transfer.TransferType != 5 || transfer.TxID != "flow-abc" || !transfer.IsDebit {
		t.Fatalf("transfer=%+v", transfer)
	}
}

func TestTransfersListFromProtoParsesCursor(t *testing.T) {
	msg := &ledgerrdv1.ListTransfersResponse{
		Transfers:     []*ledgerrdv1.TransferRow{{AssetId: 1}},
		NextPageToken: "12345",
	}
	result := decode.TransfersListFromProto(msg)
	if len(result.Transfers) != 1 {
		t.Fatalf("transfers=%+v", result.Transfers)
	}
	if result.NextCursor == nil || *result.NextCursor != 12345 {
		t.Fatalf("next_cursor=%v", result.NextCursor)
	}
}
