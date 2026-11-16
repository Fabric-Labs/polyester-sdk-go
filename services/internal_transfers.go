package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1/transferv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type InternalTransfersService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	scoped    ScopedSubAccount
}

func internalTransferAmountE18(quantity models.AssetAmountInput, quantityScale *int, assetID uint32) (*typev1.U128, error) {
	aid := assetID
	qtyBig, err := codecs.ResolveAssetAmountScaledToScale(quantity, quantityScale, codecs.LedgerScale, "quantity", models.QuantityDomainLedgerE18, &aid)
	if err != nil {
		return nil, err
	}
	return codecs.BigIntToU128Proto(qtyBig), nil
}

func NewInternalTransfersService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string) *InternalTransfersService {
	return &InternalTransfersService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *InternalTransfersService) client() transferv1connect.InternalTransferServiceClient {
	return transferv1connect.NewInternalTransferServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *InternalTransfersService) Create(ctx context.Context, assetID uint32, quantity models.AssetAmountInput, idempotencyKey string, account AccountScope, subAccountID *string, destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress *string, quantityScale *int) (models.InternalTransferResult, error) {
	dest, err := internalTransferDestination(destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress)
	if err != nil {
		return models.InternalTransferResult{}, err
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return models.InternalTransferResult{}, &errors.ValidationError{Msg: "idempotency_key is required"}
	}
	amountE18, err := internalTransferAmountE18(quantity, quantityScale, assetID)
	if err != nil {
		return models.InternalTransferResult{}, err
	}
	req := &transferv1.CreateInternalTransferRequest{
		AssetId:        assetID,
		AmountE18:      amountE18,
		IdempotencyKey: idempotencyKey,
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.InternalTransferResult{}, err
	}
	switch {
	case dest.accountID != 0:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationAccountId{DestinationAccountId: dest.accountID}
	case dest.subaccountID != 0:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSubaccountId{DestinationSubaccountId: dest.subaccountID}
	default:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSmartAccountAddress{DestinationSmartAccountAddress: dest.address}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateInternalTransfer, req, decode.InternalTransferFromProto)
}

// CreateBatch submits up to codecs.BatchInternalTransferLegsMax (10) Trading
// to Trading transfers under one idempotency key and one MFA check. Each leg
// is checked and converted as InternalTransfers.Create would, exactly, before
// anything is sent.
//
// The server checks every leg before any leg runs: if one fails, the result's
// Status is "refused", nothing moves and no step-up is used. Otherwise the
// Status is "submitted" and the legs run in order, independently: they are
// not atomic, so a leg can be refused, for example for insufficient funds,
// after earlier legs were accepted. A "pending" leg's outcome is not known
// yet: retry with the same idempotency key and the same legs in the same
// order, which answers accepted legs with the same transfers, needs no new
// step-up and runs the legs not yet accepted. Different legs under the key
// are refused with ERROR_CODE_IDEMPOTENCY_CONFLICT. Each leg counts as one
// internal transfer against the rate limit. A reply that breaks the contract
// returns a *errors.ResponseContractError; the batch may have run, so retry
// it with the same key and legs.
func (s *InternalTransfersService) CreateBatch(ctx context.Context, legs []models.InternalTransferBatchLeg, idempotencyKey string) (models.InternalTransferBatchResult, error) {
	if len(legs) < codecs.BatchInternalTransferLegsMin || len(legs) > codecs.BatchInternalTransferLegsMax {
		return models.InternalTransferBatchResult{}, &errors.ValidationError{Msg: fmt.Sprintf("a batch takes %d to %d legs, not %d", codecs.BatchInternalTransferLegsMin, codecs.BatchInternalTransferLegsMax, len(legs))}
	}
	req := &transferv1.BatchCreateInternalTransfersRequest{IdempotencyKey: idempotencyKey, Legs: make([]*transferv1.InternalTransferBatchLeg, 0, len(legs))}
	if err := codecs.CheckContractString(req, "idempotency_key", idempotencyKey, "idempotency_key"); err != nil {
		return models.InternalTransferBatchResult{}, err
	}
	for i, leg := range legs {
		wire, err := s.batchLeg(leg)
		if err != nil {
			var invalid *errors.ValidationError
			if stderrors.As(err, &invalid) {
				return models.InternalTransferBatchResult{}, &errors.ValidationError{Msg: fmt.Sprintf("legs[%d]: %s", i, invalid.Msg)}
			}
			return models.InternalTransferBatchResult{}, err
		}
		req.Legs = append(req.Legs, wire)
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().BatchCreateInternalTransfers, req, func(msg *transferv1.BatchCreateInternalTransfersResponse) (models.InternalTransferBatchResult, error) {
		return decode.InternalTransferBatchFromProto(msg, len(legs))
	})
}

func (s *InternalTransfersService) batchLeg(leg models.InternalTransferBatchLeg) (*transferv1.InternalTransferBatchLeg, error) {
	if leg.AssetID == 0 {
		return nil, &errors.ValidationError{Msg: "asset_id is required"}
	}
	dest, err := internalTransferDestination(leg.DestinationAccountID, leg.DestinationSubaccountID, leg.DestinationSmartAccountAddress)
	if err != nil {
		return nil, err
	}
	amountE18, err := internalTransferAmountE18(leg.Quantity, leg.QuantityScale, leg.AssetID)
	if err != nil {
		return nil, err
	}
	wire := &transferv1.InternalTransferBatchLeg{AssetId: leg.AssetID, AmountE18: amountE18}
	if err := s.scoped.ApplyOptionalSubaccountID(&wire.SubaccountId, leg.Account, leg.SubAccountID); err != nil {
		return nil, err
	}
	switch {
	case dest.accountID != 0:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationAccountId{DestinationAccountId: dest.accountID}
	case dest.subaccountID != 0:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationSubaccountId{DestinationSubaccountId: dest.subaccountID}
	default:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationSmartAccountAddress{DestinationSmartAccountAddress: dest.address}
	}
	return wire, nil
}

type transferDestination struct {
	accountID    uint64
	subaccountID uint64
	address      string
}

// internalTransferDestination parses the one destination of a transfer.
func internalTransferDestination(destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress *string) (transferDestination, error) {
	hasAccount := destinationAccountID != nil && strings.TrimSpace(*destinationAccountID) != ""
	hasSubaccount := destinationSubaccountID != nil && strings.TrimSpace(*destinationSubaccountID) != ""
	hasAddress := destinationSmartAccountAddress != nil && strings.TrimSpace(*destinationSmartAccountAddress) != ""
	destinations := 0
	for _, has := range []bool{hasAccount, hasSubaccount, hasAddress} {
		if has {
			destinations++
		}
	}
	if destinations != 1 {
		return transferDestination{}, &errors.ValidationError{Msg: "create requires exactly one destination"}
	}
	switch {
	case hasAccount:
		id, err := codecs.ParseAccountID(*destinationAccountID, "destination_account_id")
		return transferDestination{accountID: id}, err
	case hasSubaccount:
		id, err := codecs.ParseAccountID(*destinationSubaccountID, "destination_subaccount_id")
		return transferDestination{subaccountID: id}, err
	default:
		return transferDestination{address: *destinationSmartAccountAddress}, nil
	}
}
