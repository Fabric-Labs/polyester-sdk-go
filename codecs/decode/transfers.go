package decode

import (
	"strconv"

	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TransfersListFromProto(msg *ledgerrdv1.ListTransfersResponse) models.TransfersList {
	out := make([]models.LedgerTransfer, 0, len(msg.GetTransfers()))
	for _, t := range msg.GetTransfers() {
		out = append(out, TransferRowFromProto(t))
	}
	var cursor *int64
	if token := msg.GetNextPageToken(); token != "" {
		if parsed, err := strconv.ParseInt(token, 10, 64); err == nil && parsed != 0 {
			cursor = &parsed
		}
	}
	return models.TransfersList{Transfers: out, NextCursor: cursor}
}

// TransferRowFromProto decodes one transfer row.
func TransferRowFromProto(t *ledgerrdv1.TransferRow) models.LedgerTransfer {
	if t == nil {
		return models.LedgerTransfer{}
	}
	return models.LedgerTransfer{
		AssetID:      t.GetAssetId(),
		Amount:       u128(t.GetAmountE18()),
		TransferType: int(t.GetTransferCode()),
		AccountCode:  int(t.GetAccountCode()),
		Timestamp:    int64(t.GetTsUs()),
		TxID:         t.GetFlowId(),
		IsDebit:      t.GetIsDebit(),
	}
}
