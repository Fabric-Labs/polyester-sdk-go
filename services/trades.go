package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type TradesService struct {
	transport        *transport.Factory
	catalogs         *catalogs.Manager
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewTradesService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *TradesService {
	return &TradesService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *TradesService) client() ordersv1connect.OrdersReadServiceClient {
	return ordersv1connect.NewOrdersReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *TradesService) List(ctx context.Context, account AccountScope, subAccountID, symbol *string, symbolID *uint32, limit int, pageToken *string) (models.UserTradesList, error) {
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.UserTradesList{}, err
	}
	req := &orderv1.GetUserTradesRequest{Limit: uint32Ptr(parsedLimit)}
	if symbol != nil || symbolID != nil {
		resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "trades.list")
		if err != nil {
			return models.UserTradesList{}, err
		}
		req.SymbolId = resolved
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.UserTradesList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.UserTradesList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().GetUserTrades, req, decode.UserTradesListFromProtoChecked)
}

func (s *TradesService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.UserTrade], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:spot:trades:{account_id}:proto", accountID, s.defaultAccountID, decode.UserTradeFromBytes)
}
