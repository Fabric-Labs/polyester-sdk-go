package codecs

import (
	ledgerwritev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/write/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func ledgerWriteAmountUnits(quantity string, scale int) *ledgerwritev1.U128 {
	u := StrToU128Proto(quantity, scale)
	return &ledgerwritev1.U128{Hi: u.GetHi(), Lo: u.GetLo()}
}

func requirePositiveLedgerID(ledgerID int) error {
	if ledgerID <= 0 {
		return &errors.ValidationError{Msg: "ledger_id must be positive"}
	}
	return nil
}

// TransferTradingToTradingToProto builds a trading-to-trading transfer request.
func TransferTradingToTradingToProto(fromAccountID, toAccountID uint64, ledgerID int, quantity string, requestID *string, quantityScale int) (*ledgerwritev1.TransferTradingToTradingRequest, error) {
	if err := requirePositiveLedgerID(ledgerID); err != nil {
		return nil, err
	}
	return &ledgerwritev1.TransferTradingToTradingRequest{
		RequestId:     coalesceRequestID(requestID, "lw-ttt"),
		FromAccountId: fromAccountID,
		ToAccountId:   toAccountID,
		Ledger:        uint32(ledgerID),
		AmountUnits:   ledgerWriteAmountUnits(quantity, quantityScale),
	}, nil
}

// CreateFundingUserTransferToProto builds a funding user transfer request.
func CreateFundingUserTransferToProto(fromAccountID, toAccountID uint64, ledgerID int, quantity string, intentID *string, quantityScale int) (*ledgerwritev1.CreateFundingUserTransferRequest, error) {
	if err := requirePositiveLedgerID(ledgerID); err != nil {
		return nil, err
	}
	id := coalesceRequestID(intentID, "lw-funding")
	return &ledgerwritev1.CreateFundingUserTransferRequest{
		IntentId:      id,
		FromAccountId: fromAccountID,
		ToAccountId:   toAccountID,
		Ledger:        uint32(ledgerID),
		AmountUnits:   ledgerWriteAmountUnits(quantity, quantityScale),
	}, nil
}

// ReserveTradingWithdrawToProto builds a reserve trading withdraw request.
func ReserveTradingWithdrawToProto(accountID uint64, ledgerID int, quantity string, intentID *string, quantityScale int) (*ledgerwritev1.ReserveTradingWithdrawRequest, error) {
	if err := requirePositiveLedgerID(ledgerID); err != nil {
		return nil, err
	}
	id := coalesceRequestID(intentID, "lw-reserve")
	return &ledgerwritev1.ReserveTradingWithdrawRequest{
		IntentId:    id,
		AccountId:   accountID,
		Ledger:      uint32(ledgerID),
		AmountUnits: ledgerWriteAmountUnits(quantity, quantityScale),
	}, nil
}

// ReleaseTradingWithdrawReserveToProto builds a release trading withdraw reserve request.
func ReleaseTradingWithdrawReserveToProto(accountID uint64, ledgerID int, intentID, closeScope string) (*ledgerwritev1.ReleaseTradingWithdrawReserveRequest, error) {
	if err := requirePositiveLedgerID(ledgerID); err != nil {
		return nil, err
	}
	if intentID == "" {
		return nil, &errors.ValidationError{Msg: "intent_id is required"}
	}
	return &ledgerwritev1.ReleaseTradingWithdrawReserveRequest{
		IntentId:   intentID,
		AccountId:  accountID,
		Ledger:     uint32(ledgerID),
		CloseScope: closeScope,
	}, nil
}
