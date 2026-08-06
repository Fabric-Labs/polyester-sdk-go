package services

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1/chainwithdrawv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// PrepareAPIKeyWithdrawParams contains fields signed into an API-key withdraw.
// Omitted deadline and nonce are generated once during preparation.
type PrepareAPIKeyWithdrawParams struct {
	AssetID            uint32
	Amount             models.AssetAmountInput
	IdempotencyKey     string
	DestinationAddress string
	AmountScale        int
	DeadlineTsSec      *uint64
	Nonce              string
}

// PreparedTradingWithdraw is a complete signed request suitable for durable
// persistence and exact replay after an outcome-unknown submission.
type PreparedTradingWithdraw struct {
	request *chainwithdrawv1.CreateTradingWithdrawRequest
}

// PreparedTradingWithdrawFromBytes restores and validates persisted request bytes.
func PreparedTradingWithdrawFromBytes(data []byte) (*PreparedTradingWithdraw, error) {
	req := &chainwithdrawv1.CreateTradingWithdrawRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("invalid prepared withdraw bytes: %v", err)}
	}
	if err := validatePreparedTradingWithdraw(req); err != nil {
		return nil, err
	}
	return &PreparedTradingWithdraw{request: req}, nil
}

// Payload returns a deep copy so callers cannot mutate the prepared request.
func (p *PreparedTradingWithdraw) Payload() *chainwithdrawv1.TradingWithdrawIntentPayload {
	if p == nil || p.request == nil || p.request.Payload == nil {
		return nil
	}
	return proto.Clone(p.request.Payload).(*chainwithdrawv1.TradingWithdrawIntentPayload)
}

// PayloadSignature returns a copy of the payload signature.
func (p *PreparedTradingWithdraw) PayloadSignature() []byte {
	if p == nil || p.request == nil {
		return nil
	}
	return append([]byte(nil), p.request.PayloadSignature...)
}

// DeterministicPayloadBytes returns the exact bytes covered by the signature.
func (p *PreparedTradingWithdraw) DeterministicPayloadBytes() []byte {
	if p == nil || p.request == nil || p.request.Payload == nil {
		return nil
	}
	data, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(p.request.Payload)
	return data
}

// RequestBytes returns canonical bytes to persist before first submission.
func (p *PreparedTradingWithdraw) RequestBytes() []byte {
	if p == nil || p.request == nil {
		return nil
	}
	data, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(p.request)
	return data
}

func validatePreparedTradingWithdraw(req *chainwithdrawv1.CreateTradingWithdrawRequest) error {
	if req == nil || req.Payload == nil {
		return &errors.ValidationError{Msg: "prepared withdraw request is missing payload"}
	}
	if len(req.PayloadSignature) != ed25519.SignatureSize {
		return &errors.ValidationError{Msg: "prepared withdraw request has invalid payload_signature length"}
	}
	p := req.Payload
	if p.AssetId == 0 || p.AmountE18 == nil || p.AmountE18.Hi == 0 && p.AmountE18.Lo == 0 {
		return &errors.ValidationError{Msg: "prepared withdraw request has invalid asset_id or amount_e18"}
	}
	if p.DeadlineTsSec == 0 || p.Nonce == nil || p.Nonce.Hi == 0 && p.Nonce.Lo == 0 || p.IdempotencyKey == "" {
		return &errors.ValidationError{Msg: "prepared withdraw request is missing deadline, nonce, or idempotency_key"}
	}
	switch p.Action {
	case chainwithdrawv1.TradingWithdrawAction_TO_FUNDING:
		if p.DestinationChainId != 0 {
			return &errors.ValidationError{Msg: "prepared funding withdraw has destination_chain_id"}
		}
	case chainwithdrawv1.TradingWithdrawAction_TO_EXTERNAL_CHAIN:
		if p.DestinationChainId == 0 || p.DestinationAddress == "" {
			return &errors.ValidationError{Msg: "prepared external withdraw is missing destination chain or address"}
		}
	default:
		return &errors.ValidationError{Msg: "prepared withdraw request has unknown action"}
	}
	return nil
}

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

func defaultPreparedWithdrawDeadline() (uint64, error) {
	now := time.Now().Unix()
	if now < 0 {
		return 0, &errors.ValidationError{Msg: "system clock is before UNIX_EPOCH"}
	}
	deadline := uint64(now)
	if deadline > ^uint64(0)-300 {
		return 0, &errors.ValidationError{Msg: "withdraw deadline overflow"}
	}
	return deadline + 300, nil
}

func (s *WithdrawService) prepareAPIKey(action string, destinationChainID uint64, params PrepareAPIKeyWithdrawParams) (*PreparedTradingWithdraw, error) {
	deadline := params.DeadlineTsSec
	if deadline == nil {
		value, err := defaultPreparedWithdrawDeadline()
		if err != nil {
			return nil, err
		}
		deadline = &value
	}
	nonce := params.Nonce
	if nonce == "" {
		value, err := codecs.NewTradingWithdrawNonce()
		if err != nil {
			return nil, err
		}
		nonce = value
	}
	payload, err := codecs.TradingWithdrawPayloadToProto(action, params.AssetID, params.Amount, params.IdempotencyKey, destinationChainID, deadline, nonce, params.DestinationAddress, params.AmountScale)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := (proto.MarshalOptions{Deterministic: true}).Marshal(payload)
	if err != nil {
		return nil, &errors.ValidationError{Msg: "cannot encode withdraw payload: " + err.Error()}
	}
	creds, err := s.transport.RequireCredentials()
	if err != nil {
		return nil, err
	}
	signature, err := creds.SignPayload(payloadBytes)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedTradingWithdraw{request: &chainwithdrawv1.CreateTradingWithdrawRequest{
		Payload: payload, PayloadSignature: signature,
	}}
	if err := validatePreparedTradingWithdraw(prepared.request); err != nil {
		return nil, err
	}
	return prepared, nil
}

// PrepareAPIKeyToFunding builds and signs one complete request. Persist
// RequestBytes before submitting so an ambiguous result can reuse exact bytes.
func (s *WithdrawService) PrepareAPIKeyToFunding(params PrepareAPIKeyWithdrawParams) (*PreparedTradingWithdraw, error) {
	return s.prepareAPIKey("to_funding", 0, params)
}

// PrepareAPIKeyToExternalChain builds and signs one complete external request.
func (s *WithdrawService) PrepareAPIKeyToExternalChain(destinationChainID uint64, params PrepareAPIKeyWithdrawParams) (*PreparedTradingWithdraw, error) {
	return s.prepareAPIKey("to_external_chain", destinationChainID, params)
}

// SubmitPrepared submits the exact request that was prepared or restored.
func (s *WithdrawService) SubmitPrepared(ctx context.Context, prepared *PreparedTradingWithdraw) (models.WithdrawIntentResult, error) {
	if prepared == nil || prepared.request == nil {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "prepared withdraw is required"}
	}
	if err := validatePreparedTradingWithdraw(prepared.request); err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := proto.Clone(prepared.request).(*chainwithdrawv1.CreateTradingWithdrawRequest)
	return s.createTradingWithdraw(ctx, req)
}

// CreateAPIKeyToFunding prepares, signs, and submits once.
func (s *WithdrawService) CreateAPIKeyToFunding(ctx context.Context, params PrepareAPIKeyWithdrawParams) (models.WithdrawIntentResult, error) {
	prepared, err := s.PrepareAPIKeyToFunding(params)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	return s.SubmitPrepared(ctx, prepared)
}

// CreateAPIKeyToExternalChain prepares, signs, and submits once.
func (s *WithdrawService) CreateAPIKeyToExternalChain(ctx context.Context, destinationChainID uint64, params PrepareAPIKeyWithdrawParams) (models.WithdrawIntentResult, error) {
	prepared, err := s.PrepareAPIKeyToExternalChain(destinationChainID, params)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	return s.SubmitPrepared(ctx, prepared)
}

func (s *WithdrawService) CreateToFunding(ctx context.Context, assetID uint32, quantity models.AssetAmountInput, payloadSignature []byte, idempotencyKey string, nonce string, deadlineTsSec *uint64, account AccountScope, subAccountID *string, destinationAddress string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	payload, err := codecs.TradingWithdrawPayloadToProto("to_funding", assetID, quantity, idempotencyKey, 0, deadlineTsSec, nonce, destinationAddress, amountScale)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := &chainwithdrawv1.CreateTradingWithdrawRequest{Payload: payload, PayloadSignature: payloadSignature}
	return s.createTradingWithdraw(ctx, req)
}

func (s *WithdrawService) CreateToExternalChain(ctx context.Context, assetID uint32, quantity models.AssetAmountInput, payloadSignature []byte, destinationChainID uint64, destinationAddress string, idempotencyKey string, nonce string, deadlineTsSec *uint64, account AccountScope, subAccountID *string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	if destinationAddress == "" {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "destination_address is required for external-chain withdraw"}
	}
	payload, err := codecs.TradingWithdrawPayloadToProto("to_external_chain", assetID, quantity, idempotencyKey, destinationChainID, deadlineTsSec, nonce, destinationAddress, amountScale)
	if err != nil {
		return models.WithdrawIntentResult{}, err
	}
	req := &chainwithdrawv1.CreateTradingWithdrawRequest{Payload: payload, PayloadSignature: payloadSignature}
	return s.createTradingWithdraw(ctx, req)
}

// ValidateDestination checks one external-chain destination without creating a
// withdraw. Use it for form feedback before Trading → external submission; the
// create RPCs remain authoritative.
func (s *WithdrawService) ValidateDestination(ctx context.Context, destinationChainID uint64, destinationAddress string) (models.WithdrawDestinationValidation, error) {
	if destinationChainID == 0 {
		return models.WithdrawDestinationValidation{}, &errors.ValidationError{Msg: "destination_chain_id must be non-zero"}
	}
	if destinationAddress == "" {
		return models.WithdrawDestinationValidation{}, &errors.ValidationError{Msg: "destination_address is required"}
	}
	req := &chainwithdrawv1.ValidateWithdrawDestinationRequest{
		DestinationChainId: destinationChainID,
		DestinationAddress: destinationAddress,
	}
	return UnaryAuth(ctx, s.transport, s.client().ValidateWithdrawDestination, req, decode.WithdrawDestinationValidationFromProto)
}

func (s *WithdrawService) CreateWalletTradingWithdraw(ctx context.Context, action string, assetID uint32, amount models.AssetAmountInput, idempotencyKey string, payloadSignature []byte, signerWallet string, account AccountScope, subAccountID *string, destinationChainID uint64, deadlineTsSec *uint64, nonce string, destinationAddress string, amountScale int) (models.WithdrawIntentResult, error) {
	if len(payloadSignature) == 0 {
		return models.WithdrawIntentResult{}, &errors.ValidationError{Msg: "payload_signature is required for trading withdraw"}
	}
	payload, err := codecs.TradingWithdrawPayloadToProto(action, assetID, amount, idempotencyKey, destinationChainID, deadlineTsSec, nonce, destinationAddress, amountScale)
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
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateWalletTradingWithdraw, req, decode.WithdrawIntentFromWalletProto)
}

func (s *WithdrawService) createTradingWithdraw(ctx context.Context, req *chainwithdrawv1.CreateTradingWithdrawRequest) (models.WithdrawIntentResult, error) {
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateTradingWithdraw, req, decode.WithdrawIntentFromProto)
}
