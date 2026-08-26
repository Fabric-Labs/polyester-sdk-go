package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	feesv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1/feesv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type FeeService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	scoped    ScopedSubAccount
}

func NewFeeService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string) *FeeService {
	return &FeeService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *FeeService) client() feesv1connect.FeeServiceClient {
	return feesv1connect.NewFeeServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *FeeService) GetSpotFeeRates(ctx context.Context, account AccountScope, subAccountID *string, symbolID []uint32) (models.SpotFeeRatesList, error) {
	req := &feesv1.GetSpotFeeRatesRequest{SymbolId: symbolID}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.SpotFeeRatesList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetSpotFeeRates, req, func(msg *feesv1.GetSpotFeeRatesResponse) models.SpotFeeRatesList {
		return decode.SpotFeeRatesListFromProto(msg, s.catalogs)
	})
}
