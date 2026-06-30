package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/write/v1/ledgerwritev1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type LedgerWriteService struct {
	transport           *transport.Factory
	defaultSubAccountID *string
	defaultAccountID    *string
}

func NewLedgerWriteService(factory *transport.Factory, defaultSubAccountID, defaultAccountID *string) *LedgerWriteService {
	return &LedgerWriteService{transport: factory, defaultSubAccountID: defaultSubAccountID, defaultAccountID: defaultAccountID}
}

func (s *LedgerWriteService) client() ledgerwritev1connect.LedgerWriteServiceClient {
	return ledgerwritev1connect.NewLedgerWriteServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *LedgerWriteService) TransferTradingToTrading(ctx context.Context, toAccountID string, ledgerID int, quantity string, fromAccountID *string, requestID *string, quantityScale int) (models.LedgerWriteTransferResult, error) {
	from, err := s.resolveAccountID(fromAccountID)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	to, err := codecs.IDToInt(toAccountID, "to_account_id")
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	if quantityScale <= 0 {
		quantityScale = codecs.LedgerScale
	}
	req, err := codecs.TransferTradingToTradingToProto(from, to, ledgerID, quantity, requestID, quantityScale)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().TransferTradingToTrading, req, decode.TransferTradingToTradingFromProto)
}

func (s *LedgerWriteService) CreateFundingUserTransfer(ctx context.Context, toAccountID string, ledgerID int, quantity string, fromAccountID *string, intentID *string, quantityScale int) (models.LedgerWriteTransferResult, error) {
	from, err := s.resolveAccountID(fromAccountID)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	to, err := codecs.IDToInt(toAccountID, "to_account_id")
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	if quantityScale <= 0 {
		quantityScale = codecs.LedgerScale
	}
	req, err := codecs.CreateFundingUserTransferToProto(from, to, ledgerID, quantity, intentID, quantityScale)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateFundingUserTransfer, req, decode.CreateFundingUserTransferFromProto)
}

func (s *LedgerWriteService) ReserveTradingWithdraw(ctx context.Context, ledgerID int, quantity string, accountID *string, intentID *string, quantityScale int) (models.LedgerWriteTransferResult, error) {
	resolved, err := s.resolveAccountID(accountID)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	if quantityScale <= 0 {
		quantityScale = codecs.LedgerScale
	}
	req, err := codecs.ReserveTradingWithdrawToProto(resolved, ledgerID, quantity, intentID, quantityScale)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ReserveTradingWithdraw, req, decode.ReserveTradingWithdrawFromProto)
}

func (s *LedgerWriteService) ReleaseTradingWithdrawReserve(ctx context.Context, ledgerID int, intentID string, accountID *string, closeScope string) (models.LedgerWriteTransferResult, error) {
	resolved, err := s.resolveAccountID(accountID)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	req, err := codecs.ReleaseTradingWithdrawReserveToProto(resolved, ledgerID, intentID, closeScope)
	if err != nil {
		return models.LedgerWriteTransferResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ReleaseTradingWithdrawReserve, req, decode.ReleaseTradingWithdrawReserveFromProto)
}

func (s *LedgerWriteService) resolveAccountID(value *string) (uint64, error) {
	if value != nil && *value != "" {
		return codecs.IDToInt(*value, "account_id")
	}
	if s.defaultAccountID != nil && *s.defaultAccountID != "" {
		return codecs.IDToInt(*s.defaultAccountID, "account_id")
	}
	sub, err := ScopedSubAccountID(nil, nil, s.defaultSubAccountID)
	if err != nil {
		return 0, err
	}
	if sub != nil && *sub != "" {
		return codecs.IDToInt(*sub, "account_id")
	}
	return 0, &errors.ValidationError{Msg: "account_id is required; set POLYESTER_ACCOUNT_ID or pass account_id explicitly"}
}
