package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type BalancesService struct {
	transport        *transport.Factory
	catalogs         *catalogs.Manager
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewBalancesService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *BalancesService {
	if cats == nil {
		cats = catalogs.NewManager()
	}
	return &BalancesService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *BalancesService) client() ledgerrdv1connect.LedgerReadServiceClient {
	return ledgerrdv1connect.NewLedgerReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *BalancesService) List(ctx context.Context, account AccountScope, subAccountID *string) (models.BalancesList, error) {
	req := &ledgerrdv1.GetBalancesRequest{}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BalancesList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.BalancesList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().GetBalances, req, decode.BalancesListFromProto)
}

func (s *BalancesService) GetBalanceHistory(ctx context.Context, account AccountScope, rangeKey string, subAccountID *string, ledger uint32, accountCodes []uint32) (models.BalanceHistory, error) {
	rng, err := codecs.ResolveBalanceRange(rangeKey)
	if err != nil {
		return models.BalanceHistory{}, err
	}
	req := &ledgerrdv1.GetBalanceHistoryRequest{Range: rng, Ledger: ledger, AccountCodes: accountCodesProto(accountCodes)}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BalanceHistory{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.BalanceHistory{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().GetBalanceHistory, req, decode.BalanceHistoryFromProto)
}

func (s *BalancesService) GetEquityHistory(ctx context.Context, account AccountScope, rangeKey string, subAccountID *string, accountCodes []uint32, groupBy string) (models.EquityHistory, error) {
	rng, err := codecs.ResolveBalanceRange(rangeKey)
	if err != nil {
		return models.EquityHistory{}, err
	}
	g, err := codecs.ResolveEquityGroupBy(groupBy)
	if err != nil {
		return models.EquityHistory{}, err
	}
	req := &ledgerrdv1.GetEquityHistorySeriesRequest{Range: rng, GroupBy: g, AccountCodes: accountCodesProto(accountCodes)}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.EquityHistory{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.EquityHistory{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().GetEquityHistorySeries, req, decode.EquityHistoryFromProto)
}

func (s *BalancesService) ListHolds(ctx context.Context, account AccountScope, subAccountID *string, limit int, reversed bool) (models.HoldsList, error) {
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.HoldsList{}, err
	}
	req := &ledgerrdv1.ListHoldsRequest{Limit: parsedLimit, Reversed: reversed}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.HoldsList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.HoldsList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().ListHolds, req, decode.HoldsListFromProto)
}

func accountCodesProto(codes []uint32) []ledgerv1.AccountCode {
	if len(codes) == 0 {
		return nil
	}
	out := make([]ledgerv1.AccountCode, len(codes))
	for i, c := range codes {
		out[i] = ledgerv1.AccountCode(c)
	}
	return out
}

func (s *BalancesService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.AssetBalance], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:ledger:balances:{account_id}:proto", accountID, s.defaultAccountID, decode.AssetBalanceFromBytes)
}
