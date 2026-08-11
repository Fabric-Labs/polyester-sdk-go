package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1/marketoverviewv1connect"
	mosub "github.com/Fabric-Labs/polyester-sdk-go/marketoverview"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type MarketOverviewService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	realtime  RealtimeClient
}

// NewMarketOverviewService preserves the legacy direct-construction shape.
// Non-empty symbol filters require a catalog; root clients use
// NewMarketOverviewServiceWithCatalogs.
func NewMarketOverviewService(factory *transport.Factory, realtime RealtimeClient) *MarketOverviewService {
	return &MarketOverviewService{transport: factory, realtime: realtime}
}

func NewMarketOverviewServiceWithCatalogs(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *MarketOverviewService {
	return &MarketOverviewService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *MarketOverviewService) client() marketoverviewv1connect.MarketOverviewServiceClient {
	return marketoverviewv1connect.NewMarketOverviewServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *MarketOverviewService) List(ctx context.Context, symbols []string, limit int, _ string, includeSparklines bool) (models.MarketOverviewList, error) {
	if err := ValidateSymbolFilters(s.catalogs, symbols, "market_overview.list"); err != nil {
		return models.MarketOverviewList{}, err
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.MarketOverviewList{}, err
	}
	req := &marketoverviewv1.ListMarketOverviewRequest{Limit: parsedLimit, IncludeSparklines: includeSparklines}
	if len(symbols) > 0 {
		req.Symbols = append(req.Symbols, symbols...)
	}
	return UnaryPublic(ctx, s.transport, s.client().ListMarketOverview, req, decode.MarketOverviewListFromProto)
}

// CreateSubscriptionOptions configures a managed market overview subscription.
type MarketOverviewCreateSubscriptionOptions struct {
	Symbols           []string
	Limit             int
	IncludeSparklines bool
	OnEvent           func([]models.MarketOverviewEntry)
	OnError           func(error)
}

// CreateSubscription starts snapshot-then-stream market overview merging.
func (s *MarketOverviewService) CreateSubscription(ctx context.Context, opts MarketOverviewCreateSubscriptionOptions) (*mosub.Subscription, error) {
	if err := requireRealtime(s.realtime); err != nil {
		return nil, err
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	channel := "public:spot:market_overview:updates:proto"
	bySymbolID := map[uint32]models.MarketOverviewEntry{}

	emit := func(sub *mosub.Subscription) {
		rows := make([]models.MarketOverviewEntry, 0, len(bySymbolID))
		for _, row := range bySymbolID {
			rows = append(rows, row)
		}
		if opts.OnEvent != nil {
			opts.OnEvent(rows)
		}
		if sub != nil {
			sub.Enqueue(rows)
		}
	}

	applyRows := func(rows []models.MarketOverviewEntry) {
		for _, row := range rows {
			bySymbolID[row.SymbolID] = row
		}
	}

	var subscription *mosub.Subscription
	stream := realtime.NewSnapshotThenStream(realtime.SnapshotThenStreamConfig[models.MarketOverviewList, models.MarketOverviewList]{
		Client:  s.realtime,
		Channel: channel,
		Decode:  decode.MarketOverviewBatchFromBytes,
		FetchSnapshot: func(fetchCtx context.Context) (models.MarketOverviewList, error) {
			return s.List(fetchCtx, opts.Symbols, limit, "", opts.IncludeSparklines)
		},
		ReadPublication: func(batch models.MarketOverviewList) []models.MarketOverviewList {
			return []models.MarketOverviewList{batch}
		},
		ApplySnapshot: func(snapshot models.MarketOverviewList, buffered []models.MarketOverviewList) {
			clear(bySymbolID)
			applyRows(snapshot.Markets)
			for _, batch := range buffered {
				applyRows(batch.Markets)
			}
			emit(subscription)
		},
		ApplyLivePublications: func(batches []models.MarketOverviewList) {
			for _, batch := range batches {
				applyRows(batch.Markets)
			}
			emit(subscription)
		},
		OnError:     opts.OnError,
		MaxBuffered: 2000,
	})

	subscription = mosub.NewSubscription(stream)
	subscription.SetOnError(opts.OnError)
	if err := stream.Start(ctx); err != nil {
		subscription.Close()
		return nil, err
	}
	return subscription, nil
}

func (s *MarketOverviewService) Subscribe(ctx context.Context) (*realtime.Subscription[models.MarketOverviewList], error) {
	return SubscribePublicProto(ctx, s.realtime, "public:spot:market_overview:updates:proto", decode.MarketOverviewBatchFromBytes)
}
