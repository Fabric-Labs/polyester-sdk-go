package decode

import (
	ledgerwritev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/write/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func ledgerWriteTransferResult(transferID string, timestamp uint64) models.LedgerWriteTransferResult {
	return models.LedgerWriteTransferResult{TransferID: transferID, Timestamp: timestamp}
}

// TransferTradingToTradingFromProto decodes a trading-to-trading transfer response.
func TransferTradingToTradingFromProto(msg *ledgerwritev1.TransferTradingToTradingResponse) models.LedgerWriteTransferResult {
	return ledgerWriteTransferResult(msg.GetTransferId(), msg.GetTimestamp())
}

// CreateFundingUserTransferFromProto decodes a funding user transfer response.
func CreateFundingUserTransferFromProto(msg *ledgerwritev1.CreateFundingUserTransferResponse) models.LedgerWriteTransferResult {
	return ledgerWriteTransferResult(msg.GetTransferId(), msg.GetTimestamp())
}

// ReserveTradingWithdrawFromProto decodes a reserve trading withdraw response.
func ReserveTradingWithdrawFromProto(msg *ledgerwritev1.ReserveTradingWithdrawResponse) models.LedgerWriteTransferResult {
	return ledgerWriteTransferResult(msg.GetTransferId(), msg.GetTimestamp())
}

// ReleaseTradingWithdrawReserveFromProto decodes a release trading withdraw reserve response.
func ReleaseTradingWithdrawReserveFromProto(msg *ledgerwritev1.ReleaseTradingWithdrawReserveResponse) models.LedgerWriteTransferResult {
	return ledgerWriteTransferResult(msg.GetTransferId(), msg.GetTimestamp())
}
