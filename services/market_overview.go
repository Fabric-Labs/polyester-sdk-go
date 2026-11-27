package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
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

func NewMarketOverviewService(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *MarketOverviewService {
	if cats == nil {
		cats = catalogs.NewManager()
	}
	return &MarketOverviewService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *MarketOverviewService) client() marketoverviewv1connect.MarketOverviewServiceClient {
	return marketoverviewv1connect.NewMarketOverviewServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

// ListMarketOverviewParams selects a market overview page for ListWith.
type ListMarketOverviewParams struct {
	// Symbols lists only these markets; nil or empty lists every market.
	Symbols []string
	// Limit is the page size; 0 asks for the server's default (50).
	Limit int
	// PageToken continues from an earlier page's NextPageToken. A token is
	// bound to the symbols, OrderBy and Sort it was returned for: send it with
	// the same ones, or the server refuses it. "" reads the first page.
	PageToken string
	// IncludeSparklines asks for each row's sparklines.
	IncludeSparklines bool
	// OrderBy is the sort key: "change_24h_bps" (the absolute 24h change),
	// "volume_24h_usd" (markets without a USD volume sort last either way),
	// "last_price" or "date_added" (listing time). "" sends none: the server
	// sorts by descending 24h USD volume.
	OrderBy string
	// Sort is "asc" or "desc"; "" sends none, for the server's default.
	Sort string
	// SparklineIntervals picks the sparklines IncludeSparklines returns:
	// SPARKLINE_1H, SPARKLINE_24H, SPARKLINE_1W or SPARKLINE_1M, as
	// models.MarketOverviewSparkline.Interval reads them. Empty asks for the
	// server's default, the 24h one.
	SparklineIntervals []string
}

// List reads one overview page. The legacy string argument before
// includeSparklines is ignored; pass the returned NextPageToken as the optional
// final argument to continue with the same filters. With includeSparklines,
// each row's Sparklines holds the sparklines the server sent. Use ListWith to
// choose the sort or the sparkline intervals.
func (s *MarketOverviewService) List(ctx context.Context, symbols []string, limit int, _ string, includeSparklines bool, pageToken ...string) (models.MarketOverviewList, error) {
	if len(pageToken) > 1 {
		return models.MarketOverviewList{}, &errors.ValidationError{Msg: "at most one page token is allowed"}
	}
	params := ListMarketOverviewParams{Symbols: symbols, Limit: limit, IncludeSparklines: includeSparklines}
	if len(pageToken) == 1 {
		params.PageToken = pageToken[0]
	}
	return s.ListWith(ctx, params)
}

// ListWith reads one overview page selected by params. An unknown OrderBy,
// Sort or sparkline interval is refused with an *errors.ValidationError before
// anything is sent. Each page is cut from current values, so a market whose
// sort value changes between pages can appear twice or not at all; for a
// consistent list, read one page with the largest Limit.
func (s *MarketOverviewService) ListWith(ctx context.Context, params ListMarketOverviewParams) (models.MarketOverviewList, error) {
	parsedLimit, err := PaginationLimit(params.Limit, "limit")
	if err != nil {
		return models.MarketOverviewList{}, err
	}
	req := &marketoverviewv1.ListMarketOverviewRequest{Limit: parsedLimit, IncludeSparklines: params.IncludeSparklines, PageToken: params.PageToken}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.MarketOverviewList{}, err
	}
	if req.OrderBy, err = marketOverviewOrderBy(params.OrderBy); err != nil {
		return models.MarketOverviewList{}, err
	}
	if req.Sort, err = marketOverviewSort(params.Sort); err != nil {
		return models.MarketOverviewList{}, err
	}
	for _, label := range params.SparklineIntervals {
		interval, err := marketOverviewSparklineInterval(label)
		if err != nil {
			return models.MarketOverviewList{}, err
		}
		req.SparklineIntervals = append(req.SparklineIntervals, interval)
	}
	symbols := params.Symbols
	// Rows are labelled and scaled from the catalogs; only a symbol filter needs them.
	if err := awaitCatalogs(ctx, s.catalogs, namesSymbol(nil, symbols...)); err != nil {
		return models.MarketOverviewList{}, err
	}
	for _, symbol := range symbols {
		trimmed := strings.TrimSpace(symbol)
		if trimmed == "" {
			continue
		}
		id, err := ResolveSymbolID(s.catalogs, &trimmed, nil, "market_overview.list")
		if err != nil {
			return models.MarketOverviewList{}, err
		}
		req.SymbolId = append(req.SymbolId, id)
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListMarketOverview, req, func(msg *marketoverviewv1.ListMarketOverviewResponse) (models.MarketOverviewList, error) {
		return decode.MarketOverviewListFromProto(msg, s.catalogs)
	})
}

func marketOverviewOrderBy(label string) (marketoverviewv1.MarketOrderBy, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return marketoverviewv1.MarketOrderBy_MARKET_ORDER_BY_UNSPECIFIED, nil
	case "change_24h_bps":
		return marketoverviewv1.MarketOrderBy_ORDER_BY_CHANGE_24H_BPS, nil
	case "volume_24h_usd":
		return marketoverviewv1.MarketOrderBy_ORDER_BY_VOLUME_24H_USD, nil
	case "last_price":
		return marketoverviewv1.MarketOrderBy_ORDER_BY_LAST_PRICE, nil
	case "date_added":
		return marketoverviewv1.MarketOrderBy_ORDER_BY_DATE_ADDED, nil
	default:
		return 0, &errors.ValidationError{Msg: "market_overview.list order_by must be change_24h_bps, volume_24h_usd, last_price or date_added, or empty for the server's default, not " + label}
	}
}

func marketOverviewSort(label string) (marketoverviewv1.SortDirection, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return marketoverviewv1.SortDirection_SORT_DIRECTION_UNSPECIFIED, nil
	case "asc":
		return marketoverviewv1.SortDirection_SORT_ASC, nil
	case "desc":
		return marketoverviewv1.SortDirection_SORT_DESC, nil
	default:
		return 0, &errors.ValidationError{Msg: "market_overview.list sort must be asc or desc, or empty for the server's default, not " + label}
	}
}

func marketOverviewSparklineInterval(label string) (marketoverviewv1.SparklineInterval, error) {
	switch strings.TrimSpace(label) {
	case "SPARKLINE_1H":
		return marketoverviewv1.SparklineInterval_SPARKLINE_1H, nil
	case "SPARKLINE_24H":
		return marketoverviewv1.SparklineInterval_SPARKLINE_24H, nil
	case "SPARKLINE_1W":
		return marketoverviewv1.SparklineInterval_SPARKLINE_1W, nil
	case "SPARKLINE_1M":
		return marketoverviewv1.SparklineInterval_SPARKLINE_1M, nil
	default:
		return 0, &errors.ValidationError{Msg: "market_overview.list sparkline interval must be SPARKLINE_1H, SPARKLINE_24H, SPARKLINE_1W or SPARKLINE_1M, not " + label}
	}
}

const maxSpotVolumeHistorySymbolIDs = 2000

// GetSpotVolumeHistory returns trailing 24-hour USD volume samples.
// Omit both filters to select every configured pair. At most 2,000 distinct
// positive pair IDs are accepted. Do not sum the overlapping samples.
func (s *MarketOverviewService) GetSpotVolumeHistory(ctx context.Context, symbols []string, symbolIDs []uint32) (models.SpotVolumeHistory, error) {
	if len(symbols) > 0 && len(symbolIDs) > 0 {
		return models.SpotVolumeHistory{}, &errors.ValidationError{
			Msg: "market_overview.get_spot_volume_history accepts only one of symbols or symbol_ids",
		}
	}
	// Pairs are labelled from the catalogs; only a symbol filter needs them.
	if err := awaitCatalogs(ctx, s.catalogs, namesSymbol(nil, symbols...)); err != nil {
		return models.SpotVolumeHistory{}, err
	}
	resolved := make([]uint32, 0, len(symbolIDs)+len(symbols))
	seen := make(map[uint32]struct{}, len(symbolIDs)+len(symbols))
	if len(symbolIDs) > 0 {
		for _, id := range symbolIDs {
			if id == 0 {
				return models.SpotVolumeHistory{}, &errors.ValidationError{
					Msg: "market_overview.get_spot_volume_history symbol_ids must be positive",
				}
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			resolved = append(resolved, id)
		}
	} else {
		for _, symbol := range symbols {
			trimmed := strings.TrimSpace(symbol)
			if trimmed == "" {
				continue
			}
			id, err := ResolveSymbolID(s.catalogs, &trimmed, nil, "market_overview.get_spot_volume_history symbols")
			if err != nil {
				return models.SpotVolumeHistory{}, err
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			resolved = append(resolved, id)
		}
	}
	if len(resolved) > maxSpotVolumeHistorySymbolIDs {
		return models.SpotVolumeHistory{}, &errors.ValidationError{
			Msg: "market_overview.get_spot_volume_history accepts at most 2000 symbol_ids",
		}
	}
	req := &marketoverviewv1.GetSpotVolumeHistoryRequest{SymbolId: resolved}
	return UnaryPublic(ctx, s.transport, s.client().GetSpotVolumeHistory, req, func(msg *marketoverviewv1.GetSpotVolumeHistoryResponse) models.SpotVolumeHistory {
		return decode.SpotVolumeHistoryFromProto(msg, s.catalogs)
	})
}

// GetCurrencyConversionConfig returns supported fiat and stablecoin display metadata.
// Entries are ordered by code. Names, symbols, and fraction digits are
// presentation defaults; fraction digits do not specify rate precision.
// Configuration remains available before rates are observed.
func (s *MarketOverviewService) GetCurrencyConversionConfig(ctx context.Context) (models.CurrencyConversionConfig, error) {
	return UnaryPublic(ctx, s.transport, s.client().GetCurrencyConversionConfig, &marketoverviewv1.GetCurrencyConversionConfigRequest{}, decode.CurrencyConversionConfigFromProto)
}

// GetCurrencyConversionRates returns fiat units per USD and USD per stablecoin unit.
// Fiat UnitsPerUsdE8 is currency units per 1 USD at 1e8 scale (USD identity is
// 100_000_000). Stablecoin UsdPerUnitE8 is observed USD per unit at the same
// scale. A missing fiat snapshot or omitted stablecoin is unobserved, not zero.
// The request fails with unavailable (HTTP 503) before any observation exists.
func (s *MarketOverviewService) GetCurrencyConversionRates(ctx context.Context) (models.CurrencyConversionRates, error) {
	return UnaryPublic(ctx, s.transport, s.client().GetCurrencyConversionRates, &marketoverviewv1.GetCurrencyConversionRatesRequest{}, decode.CurrencyConversionRatesFromProto)
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
// With Symbols set, only those markets are emitted, from the snapshot and from
// live and buffered publications alike.
func (s *MarketOverviewService) CreateSubscription(ctx context.Context, opts MarketOverviewCreateSubscriptionOptions) (*mosub.Subscription, error) {
	if err := requireRealtime(s.realtime); err != nil {
		return nil, err
	}
	// Rows are labelled and scaled from the catalogs; only a symbol filter needs them.
	if err := awaitCatalogs(ctx, s.catalogs, namesSymbol(nil, opts.Symbols...)); err != nil {
		return nil, err
	}
	selected := map[uint32]struct{}{}
	for _, symbol := range opts.Symbols {
		trimmed := strings.TrimSpace(symbol)
		if trimmed == "" {
			continue
		}
		id, err := ResolveSymbolID(s.catalogs, &trimmed, nil, "market_overview.create_subscription")
		if err != nil {
			return nil, err
		}
		selected[id] = struct{}{}
	}
	matches := func(row models.MarketOverviewEntry) bool {
		if len(selected) == 0 {
			return true
		}
		_, ok := selected[row.SymbolID]
		return ok
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	channel := "public:spot:market_overview:updates:proto"
	bySymbolID := map[uint32]models.MarketOverviewEntry{}
	decodeBatch := decode.MarketOverviewBatchDecoder(s.catalogs)

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
			if matches(row) {
				// Live publications carry no sparklines: keep the snapshot's.
				if previous, ok := bySymbolID[row.SymbolID]; ok && len(row.Sparklines) == 0 {
					row.Sparklines = previous.Sparklines
				}
				bySymbolID[row.SymbolID] = row
			}
		}
	}

	var subscription *mosub.Subscription
	stream := realtime.NewSnapshotThenStream(realtime.SnapshotThenStreamConfig[models.MarketOverviewList, models.MarketOverviewList]{
		Client:  s.realtime,
		Channel: channel,
		Decode:  decodeBatch,
		FetchSnapshot: func(fetchCtx context.Context) (models.MarketOverviewList, error) {
			return s.List(fetchCtx, opts.Symbols, limit, "", opts.IncludeSparklines)
		},
		ReadPublication: func(batch models.MarketOverviewList) []models.MarketOverviewList {
			markets := make([]models.MarketOverviewEntry, 0, len(batch.Markets))
			for _, row := range batch.Markets {
				if matches(row) {
					markets = append(markets, row)
				}
			}
			if len(markets) == 0 {
				return nil
			}
			return []models.MarketOverviewList{{Markets: markets}}
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
	// Rows are labelled and scaled from the catalogs.
	if err := awaitCatalogs(ctx, s.catalogs, false); err != nil {
		return nil, err
	}
	return SubscribePublicProto(ctx, s.realtime, "public:spot:market_overview:updates:proto", decode.MarketOverviewBatchDecoder(s.catalogs))
}
