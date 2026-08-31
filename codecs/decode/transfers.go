package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TransferSideKindLabel(kind ledgerrdv1.TransferSideKind) string {
	switch kind {
	case ledgerrdv1.TransferSideKind_FUNDING_ACCOUNT:
		return "funding_account"
	case ledgerrdv1.TransferSideKind_TRADING_ACCOUNT:
		return "trading_account"
	case ledgerrdv1.TransferSideKind_EXTERNAL_ADDRESS:
		return "external_address"
	case ledgerrdv1.TransferSideKind_PRIVATE_COUNTERPARTY:
		return "private_counterparty"
	case ledgerrdv1.TransferSideKind_FEE_ACCOUNT:
		return "fee_account"
	case ledgerrdv1.TransferSideKind_SYSTEM_ACCOUNT:
		return "system_account"
	default:
		return ""
	}
}

func TransferSideFromProto(msg *ledgerrdv1.TransferSide) *models.TransferSide {
	if msg == nil {
		return nil
	}
	side := &models.TransferSide{
		Kind:    TransferSideKindLabel(msg.GetKind()),
		Address: msg.GetAddress(),
	}
	if msg.AccountId != nil {
		side.AccountID = codecs.FormatID(*msg.AccountId)
	}
	if msg.ChainId != nil {
		id := *msg.ChainId
		side.ChainID = &id
	}
	if side.Kind == "" && side.AccountID == "" && side.Address == "" && side.ChainID == nil {
		return nil
	}
	return side
}

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
		Source:       TransferSideFromProto(t.GetSource()),
		Destination:  TransferSideFromProto(t.GetDestination()),
	}
}
