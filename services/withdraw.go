package services

import (
	"context"
	"fmt"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1/chainwithdrawv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type WithdrawService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewWithdrawService(factory *transport.Factory, defaultSubAccountID *string) *WithdrawService {
	return &WithdrawService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *WithdrawService) client() chainwithdrawv1connect.WithdrawServiceClient {
	return chainwithdrawv1connect.NewWithdrawServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *WithdrawService) CreateToFunding(ctx context.Context, assetID uint32, quantity string, payloadSignature []byte, idempotencyKey *string, account AccountScope, subAccountID *string, destinationAddress string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	key := codecs.NewTradingWithdrawIdempotencyKey()
	if idempotencyKey != nil {
		key = *idempotencyKey
	}
	payload, err := codecs.TradingWithdrawPayloadToProto("to_funding", assetID, quantity, key, 0, nil, nil, destinationAddress, amountScale)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := &chainwithdrawv1.CreateTradingWithdrawRequest{Payload: payload, PayloadSignature: payloadSignature}
	return s.createTradingWithdraw(ctx, req)
}

func (s *WithdrawService) CreateToExternalChain(ctx context.Context, assetID uint32, quantity string, payloadSignature []byte, destinationChainID uint64, destinationAddress string, idempotencyKey *string, account AccountScope, subAccountID *string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	if destinationAddress == "" {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "destination_address is required for external-chain withdraw"}
	}
	key := codecs.NewTradingWithdrawIdempotencyKey()
	if idempotencyKey != nil {
		key = *idempotencyKey
	}
	payload, err := codecs.TradingWithdrawPayloadToProto("to_external_chain", assetID, quantity, key, destinationChainID, nil, nil, destinationAddress, amountScale)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := &chainwithdrawv1.CreateTradingWithdrawRequest{Payload: payload, PayloadSignature: payloadSignature}
	return s.createTradingWithdraw(ctx, req)
}

func (s *WithdrawService) CreateWalletTradingWithdraw(ctx context.Context, action string, assetID uint32, amount, idempotencyKey string, payloadSignature []byte, signerWallet string, account AccountScope, subAccountID *string, destinationChainID uint64, deadlineTsSec *uint64, nonce any, destinationAddress string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	var nonceStr *string
	if nonce != nil {
		s := fmt.Sprint(nonce)
		nonceStr = &s
	}
	payload, err := codecs.TradingWithdrawPayloadToProto(action, assetID, amount, idempotencyKey, destinationChainID, deadlineTsSec, nonceStr, destinationAddress, amountScale)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := &chainwithdrawv1.CreateWalletTradingWithdrawRequest{
		Payload:          payload,
		SignerWallet:     signerWallet,
		PayloadSignature: payloadSignature,
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.WithdrawIntentResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateWalletTradingWithdraw, req, decode.WithdrawIntentFromWalletProto)
}

func (s *WithdrawService) createTradingWithdraw(ctx context.Context, req *chainwithdrawv1.CreateTradingWithdrawRequest) (models.WithdrawIntentResult, error) {
	return UnaryAuth(ctx, s.transport, s.client().CreateTradingWithdraw, req, decode.WithdrawIntentFromProto)
}
