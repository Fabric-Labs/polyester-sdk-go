package decode_test

import (
	"encoding/json"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// transferJSON is the decoded row as a caller serialises it; decoding through
// JSON keeps this test independent of the Go field types it checks.
func transferJSON(t *testing.T, transfer models.LedgerTransfer) map[string]any {
	t.Helper()
	raw, err := json.Marshal(transfer)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Both values lie beyond float64's exact integer range: 2^64+5 and 2^63+1.
func populatedTransferRow() *ledgerrdv1.TransferRow {
	return &ledgerrdv1.TransferRow{
		AssetId:         2,
		AmountE18:       &typev1.U128{Lo: 1000},
		BalanceAfterE18: &typev1.U128{Hi: 1, Lo: 5},
		LinkId:          9223372036854775809,
		TsUs:            999,
		TransferId:      "42",
	}
}

func TestTransferReadAndStreamKeepBalanceAfterAndLinkID(t *testing.T) {
	row := populatedTransferRow()
	payload, err := proto.Marshal(&ledgerrdv1.TransferRowBatch{Events: []*ledgerrdv1.TransferRow{row}})
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := decode.LedgerTransferBatchFromBytes(payload)
	if err != nil || len(streamed) != 1 {
		t.Fatalf("stream decode = %v, %v", streamed, err)
	}
	listed := decode.TransfersListFromProto(&ledgerrdv1.ListTransfersResponse{Transfers: []*ledgerrdv1.TransferRow{row}})
	for path, transfer := range map[string]models.LedgerTransfer{
		"list":   listed.Transfers[0],
		"stream": streamed[0],
	} {
		got := transferJSON(t, transfer)
		if got["balance_after"] != "18446744073709551621" {
			t.Errorf("%s: balance_after = %#v, want the exact E18 balance", path, got["balance_after"])
		}
		if got["link_id"] != "9223372036854775809" {
			t.Errorf("%s: link_id = %#v, want the exact link ID", path, got["link_id"])
		}
	}
}

func TestTransferBalanceAfterZeroIsNotAbsent(t *testing.T) {
	zero := populatedTransferRow()
	zero.BalanceAfterE18 = &typev1.U128{}
	zero.LinkId = 0
	got := transferJSON(t, decode.TransferRowFromProto(zero))
	if got["balance_after"] != "0" {
		t.Errorf("a zero post-transfer balance must read \"0\", got %#v", got["balance_after"])
	}
	// The contract sends link_id 0 when no correlation ID exists.
	if value, ok := got["link_id"]; ok {
		t.Errorf("link_id 0 means unavailable and must be omitted, got %#v", value)
	}

	absent := populatedTransferRow()
	absent.BalanceAfterE18 = nil
	got = transferJSON(t, decode.TransferRowFromProto(absent))
	if value, ok := got["balance_after"]; ok {
		t.Errorf("an absent post-transfer balance must be omitted, got %#v", value)
	}
}
