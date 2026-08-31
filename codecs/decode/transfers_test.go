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

func TestTransferRowFromProtoMapsExternalSideChainID(t *testing.T) {
	accountID := uint64(11)
	chainID := uint32(8453)
	msg := &ledgerrdv1.TransferRow{
		AssetId:      2,
		AmountE18:    &typev1.U128{Lo: 1000},
		TransferCode: 5,
		AccountCode:  1,
		TsUs:         999,
		FlowId:       "flow-abc",
		Source: &ledgerrdv1.TransferSide{
			Kind:      ledgerrdv1.TransferSideKind_FUNDING_ACCOUNT,
			AccountId: &accountID,
			Address:   "0x1111111111111111111111111111111111111111",
		},
		Destination: &ledgerrdv1.TransferSide{
			Kind:    ledgerrdv1.TransferSideKind_EXTERNAL_ADDRESS,
			Address: "0x2222222222222222222222222222222222222222",
			ChainId: &chainID,
		},
	}
	transfer := decode.TransferRowFromProto(msg)
	if transfer.Source == nil || transfer.Source.Kind != "funding_account" || transfer.Source.AccountID == "" {
		t.Fatalf("source=%+v", transfer.Source)
	}
	if transfer.Destination == nil || transfer.Destination.Kind != "external_address" {
		t.Fatalf("destination=%+v", transfer.Destination)
	}
	if transfer.Destination.ChainID == nil || *transfer.Destination.ChainID != 8453 {
		t.Fatalf("destination.chain_id=%v", transfer.Destination.ChainID)
	}
	if transfer.Source.ChainID != nil {
		t.Fatalf("funding side must omit zipper chain_id: %+v", transfer.Source)
	}
}

func TestTransferSideFromProtoOmitsEmptyAndUnsetChainID(t *testing.T) {
	if got := decode.TransferSideFromProto(nil); got != nil {
		t.Fatalf("nil proto=%+v", got)
	}
	if got := decode.TransferSideFromProto(&ledgerrdv1.TransferSide{}); got != nil {
		t.Fatalf("empty proto=%+v", got)
	}
	side := decode.TransferSideFromProto(&ledgerrdv1.TransferSide{
		Kind:    ledgerrdv1.TransferSideKind_EXTERNAL_ADDRESS,
		Address: "0xabc",
	})
	if side == nil || side.ChainID != nil {
		t.Fatalf("unset chain_id should stay omitted: %+v", side)
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
