package services

import (
	"context"
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
	destinations := 0
	if destinationAccountID != nil && strings.TrimSpace(*destinationAccountID) != "" {
		destinations++
	}
	if destinationSubaccountID != nil && strings.TrimSpace(*destinationSubaccountID) != "" {
		destinations++
	}
	if destinationSmartAccountAddress != nil && strings.TrimSpace(*destinationSmartAccountAddress) != "" {
		destinations++
	}
	if destinations != 1 {
		return models.InternalTransferResult{}, &errors.ValidationError{Msg: "create requires exactly one destination"}
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
	if destinationAccountID != nil {
		id, err := codecs.IDToInt(*destinationAccountID, "destination_account_id")
		if err != nil {
			return models.InternalTransferResult{}, err
		}
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationAccountId{DestinationAccountId: id}
	}
	if destinationSubaccountID != nil {
		id, err := codecs.IDToInt(*destinationSubaccountID, "destination_subaccount_id")
		if err != nil {
			return models.InternalTransferResult{}, err
		}
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSubaccountId{DestinationSubaccountId: id}
	}
	if destinationSmartAccountAddress != nil {
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSmartAccountAddress{DestinationSmartAccountAddress: *destinationSmartAccountAddress}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateInternalTransfer, req, decode.InternalTransferFromProto)
}
