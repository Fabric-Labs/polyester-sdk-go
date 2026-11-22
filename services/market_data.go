package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// timeframeAliases maps each documented candle timeframe label to its
// Timeframe enum name.
var timeframeAliases = map[string]string{"1s": "SEC_1", "1m": "MIN_1", "5m": "MIN_5", "15m": "MIN_15", "30m": "MIN_30", "1h": "HOUR_1", "4h": "HOUR_4", "12h": "HOUR_12", "1d": "DAY_1", "1w": "WEEK_1", "1mo": "MONTH_1", "3m": "MIN_3", "2h": "HOUR_2", "6h": "HOUR_6", "8h": "HOUR_8", "3d": "DAY_3"}

// candleTimeframes maps each accepted candle timeframe, exactly as written, to
// its Timeframe: a documented label ("1s", "1m", "3m", ... "1w", "1mo") or an
// enum name ("SEC_1", "MIN_1", ... "MONTH_1"). No other case or spelling is
// read, so "1M" is never taken for "1m" (one minute) nor for "1mo" (one month).
var candleTimeframes = func() map[string]marketdatav1.Timeframe {
	out := make(map[string]marketdatav1.Timeframe, len(timeframeAliases)*2)
	for label, enumName := range timeframeAliases {
		value := marketdatav1.Timeframe(marketdatav1.Timeframe_value[enumName])
		out[label] = value
		out[enumName] = value
	}
	return out
}()

// candleChannelLabels names each timeframe as live Centrifugo channels do: by
// its label (`1m`), not its enum name (`MIN_1`).
var candleChannelLabels = func() map[marketdatav1.Timeframe]string {
	out := make(map[marketdatav1.Timeframe]string, len(timeframeAliases))
	for label, enumName := range timeframeAliases {
		out[marketdatav1.Timeframe(marketdatav1.Timeframe_value[enumName])] = label
	}
	return out
}()

// resolveCandleTimeframe reads a candle timeframe, a label or an enum name
// exactly as written (see candleTimeframes); anything else is a
// ValidationError.
func resolveCandleTimeframe(timeframe string) (marketdatav1.Timeframe, error) {
	if value, ok := candleTimeframes[timeframe]; ok {
		return value, nil
	}
	return marketdatav1.Timeframe_TIMEFRAME_UNSPECIFIED, &errors.ValidationError{Msg: fmt.Sprintf(
		"Unknown candle timeframe %q; use a label such as '1m', '1h', '1d' or '1mo' (one month), or an enum name such as 'MIN_1'", timeframe,
	)}
}

type MarketDataService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	realtime  RealtimeClient
}

func NewMarketDataService(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *MarketDataService {
	return &MarketDataService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *MarketDataService) client() marketdatav1connect.MarketDataServiceClient {
	return marketdatav1connect.NewMarketDataServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *MarketDataService) GetSpotConfig(ctx context.Context) (models.SpotConfig, error) {
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetSpotConfig, &marketdatav1.GetSpotConfigRequest{}, decode.SpotConfigFromProto)
}

// ListAssetInfo returns the team-maintained information and market
// datapoints of the given assets (all saved entries when assetIDs is empty),
// in ascending asset ID order. Asset IDs are the LedgerID of the spot and
// funding configs; duplicate and unknown IDs are ignored, and assets with
// neither team-written information nor market datapoints are left out: an
// asset listed for its MarketStats alone has an empty Editorial. Each ID is
// 1 to 2147483647 and at most 1000 are taken (codecs.AssetInfoIDMin,
// AssetInfoIDMax and AssetInfoIDsMax); others are refused with an
// *errors.ValidationError before sending. It is public: the request is
// never signed. The result is the complete snapshot,
// with no paging.
func (s *MarketDataService) ListAssetInfo(ctx context.Context, assetIDs ...uint32) ([]models.AssetInfo, error) {
	if err := codecs.CheckAssetInfoIDs(assetIDs); err != nil {
		return nil, err
	}
	req := &marketdatav1.ListAssetInfoRequest{AssetIds: append([]uint32(nil), assetIDs...)}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListAssetInfo, req, decode.AssetInfoListFromProto)
}

func (s *MarketDataService) GetTrades(ctx context.Context, symbol *string, symbolID *uint32, limit int, pageToken *string) (models.MarketTradesResult, error) {
	// Decoded with the pair's catalog scales.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return models.MarketTradesResult{}, err
	}
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "get_trades")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	scale, err := s.requireQuantityScale(resolved, "get_trades")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	req := &marketdatav1.GetTradesRequest{SymbolId: resolved, Limit: parsedLimit}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.MarketTradesResult{}, err
	}
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	return UnaryPublic(ctx, s.transport, s.client().GetTrades, req, func(msg *marketdatav1.GetTradesResponse) models.MarketTradesResult {
		return decode.MarketTradesFromProto(msg, scale)
	})
}

// GetTradesAfter walks a symbol's public trades gap-free: the trades with a
// match ID greater than afterTradeID (a decimal match ID; "0" starts from the
// symbol's first trade), ascending, up to limit, and only up to the result's
// CompleteThroughTradeID. Pass the result's NextAfterTradeID to the next
// call; when it equals afterTradeID, nothing newer is available yet, so
// retry later with backoff. The walk crosses epoch ends without a gap and
// reports them in EpochEnds. It cannot be combined with GetTrades' page
// token, time range or side filter.
func (s *MarketDataService) GetTradesAfter(ctx context.Context, symbol *string, symbolID *uint32, afterTradeID string, limit int) (models.MarketTradesResult, error) {
	after, err := codecs.ParseRequiredUint64Decimal(afterTradeID, "after_trade_id")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	// Decoded with the pair's catalog scales.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return models.MarketTradesResult{}, err
	}
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "get_trades_after")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	scale, err := s.requireQuantityScale(resolved, "get_trades_after")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.MarketTradesResult{}, err
	}
	req := &marketdatav1.GetTradesRequest{SymbolId: resolved, Limit: parsedLimit, AfterTradeId: &after}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.MarketTradesResult{}, err
	}
	return UnaryPublic(ctx, s.transport, s.client().GetTrades, req, func(msg *marketdatav1.GetTradesResponse) models.MarketTradesResult {
		return decode.MarketTradesBackfillFromProto(msg, scale)
	})
}

// GetCandles returns OHLCV candles newest-first. When includeIncomplete is true,
// the open candle, if present, is prepended.
func (s *MarketDataService) GetCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken ...string) (models.CandlesResult, error) {
	return s.GetCandlesWithReference(ctx, symbol, symbolID, timeframe, limit, start, end, includeIncomplete, false, pageToken...)
}

// GetCandlesWithReference returns OHLCV candles and, when includeReference is
// true, composite reference candles. Primary OHLC uses the fixed Q9 price scale. Reference
// OHLC uses the pair reference_price_scale. Base volume uses the base asset
// market_data_volume_scale.
func (s *MarketDataService) GetCandlesWithReference(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, includeReference bool, pageToken ...string) (models.CandlesResult, error) {
	if len(pageToken) > 1 {
		return models.CandlesResult{}, &errors.ValidationError{Msg: "at most one page token is allowed"}
	}
	var token *string
	if len(pageToken) == 1 {
		token = &pageToken[0]
	}

	req, scale, refScale, err := s.buildCandlesRequest(ctx, symbol, symbolID, timeframe, limit, start, end, includeIncomplete, token, includeReference)
	if err != nil {
		return models.CandlesResult{}, err
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetCandles, req, func(msg *marketdatav1.GetCandlesResponse) (models.CandlesResult, error) {
		priceScale, err := referencePriceScaleForDecode(refScale, len(msg.GetReferenceCandles()) > 0, req.SymbolId)
		if err != nil {
			return models.CandlesResult{}, err
		}
		return decode.CandlesFromProto(msg, scale, priceScale)
	})
}

// GetCurrentCandle returns the latest candle for a symbol/timeframe, or nil when
// the market has no candle rows (aligns with Rust Option<Candle>).
func (s *MarketDataService) GetCurrentCandle(ctx context.Context, symbol *string, symbolID *uint32, timeframe string) (*models.Candle, error) {
	result, err := s.GetCandles(ctx, symbol, symbolID, timeframe, 1, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if len(result.Candles) == 0 {
		return nil, nil
	}
	candle := currentCandle(result.Candles)
	return &candle, nil
}

func currentCandle(candles []models.Candle) models.Candle {
	return candles[0]
}

// GetCandlesColumns returns OHLCV candles in columnar wire form, decoded to rows.
// Each row's Candle.IsClosed is the server's is_closed flag, as GetCandles
// reports it: with includeIncomplete, the current open candle is appended last
// on the first page and is the only row that reads false. From a server that
// predates the flag, every row reads true except the newest row of a first
// page (nil or empty pageToken) read with includeIncomplete, which reads
// false, meaning not known to be closed.
func (s *MarketDataService) GetCandlesColumns(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken *string) (models.CandlesResult, error) {
	return s.GetCandlesColumnsWithReference(ctx, symbol, symbolID, timeframe, limit, start, end, includeIncomplete, pageToken, false)
}

// GetCandlesColumnsWithReference returns columnar OHLCV decoded to rows, including
// reference candles when includeReference is true. Primary rows' Candle.IsClosed
// is read as GetCandlesColumns reads it; every reference row is a closed
// bucket and reads true.
func (s *MarketDataService) GetCandlesColumnsWithReference(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken *string, includeReference bool) (models.CandlesResult, error) {
	base, scale, refScale, err := s.buildCandlesRequest(ctx, symbol, symbolID, timeframe, limit, start, end, includeIncomplete, pageToken, includeReference)
	if err != nil {
		return models.CandlesResult{}, err
	}
	req := &marketdatav1.GetCandlesColumnsRequest{
		SymbolId: base.SymbolId, Timeframe: base.Timeframe, Limit: base.Limit,
		StartTime: base.StartTime, EndTime: base.EndTime, IncludeIncomplete: base.IncludeIncomplete,
		IncludeReference: base.IncludeReference, PageToken: base.PageToken,
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetCandlesColumns, req, func(msg *marketdatav1.GetCandlesColumnsResponse) (models.CandlesResult, error) {
		hasReference := len(msg.GetReferenceTsSec()) > 0 || len(msg.GetReferenceOpen()) > 0 ||
			len(msg.GetReferenceHigh()) > 0 || len(msg.GetReferenceLow()) > 0 ||
			len(msg.GetReferenceClose()) > 0 || len(msg.GetReferenceVolume()) > 0
		priceScale, err := referencePriceScaleForDecode(refScale, hasReference, req.SymbolId)
		if err != nil {
			return models.CandlesResult{}, err
		}
		return decode.CandlesColumnsFromProto(msg, scale, priceScale, req.GetIncludeIncomplete(), req.GetPageToken())
	})
}

func (s *MarketDataService) buildCandlesRequest(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken *string, includeReference bool) (*marketdatav1.GetCandlesRequest, int, int, error) {
	// An unknown timeframe is refused before the catalog wait or any request.
	tf, err := resolveCandleTimeframe(timeframe)
	if err != nil {
		return nil, 0, 0, err
	}
	// Decoded with the pair's catalog scales.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return nil, 0, 0, err
	}
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "get_candles")
	if err != nil {
		return nil, 0, 0, err
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return nil, 0, 0, err
	}
	req := &marketdatav1.GetCandlesRequest{SymbolId: resolved, Timeframe: tf, Limit: parsedLimit, IncludeIncomplete: includeIncomplete, IncludeReference: includeReference}
	if err := codecs.CheckPageLimit(req); err != nil {
		return nil, 0, 0, err
	}
	if start != nil {
		req.StartTime = timestamppb.New(*start)
	}
	if end != nil {
		req.EndTime = timestamppb.New(*end)
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	scale, err := s.requireMarketDataVolumeScale(resolved, "candle volume")
	if err != nil {
		return nil, 0, 0, err
	}
	refScale, refOK := 0, false
	if s.catalogs != nil {
		refScale, refOK = s.catalogs.ReferencePriceScaleForSymbolID(resolved)
	}
	if includeReference && !refOK {
		return nil, 0, 0, &errors.ValidationError{
			Msg: fmt.Sprintf("candle reference prices require reference_price_scale for symbol_id %d", resolved),
		}
	}
	if !refOK {
		refScale = -1
	}
	return req, scale, refScale, nil
}

func (s *MarketDataService) SubscribeTrades(ctx context.Context, symbol *string, symbolID *uint32) (*realtime.Subscription[models.MarketTrade], error) {
	// Decoded with the pair's catalog scales.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return nil, err
	}
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_trades")
	if err != nil {
		return nil, err
	}
	scale, err := s.requireQuantityScale(resolved, "subscribe_trades")
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:market:trades:%d:proto", resolved)
	return SubscribePublicProtoBatch(ctx, s.realtime, channel, decode.MarketTradeBatchFromBytes(scale))
}

func (s *MarketDataService) SubscribeCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string) (*realtime.Subscription[models.Candle], error) {
	// An unknown timeframe is refused before the catalog wait or subscribing.
	channelTimeframe, err := resolveCandleChannelTimeframe(timeframe)
	if err != nil {
		return nil, err
	}
	// Decoded with the pair's catalog scales.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return nil, err
	}
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_candles")
	if err != nil {
		return nil, err
	}
	volumeScale, err := s.requireMarketDataVolumeScale(resolved, "candle volume")
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:market:candles:%s:%d:proto", channelTimeframe, resolved)
	decodeFn := decode.CandlePointFromBytes(resolved, channelTimeframe, volumeScale)
	return SubscribePublicProto(ctx, s.realtime, channel, decodeFn)
}

func referencePriceScaleForDecode(scale int, hasReference bool, symbolID uint32) (int, error) {
	if scale >= 0 {
		return scale, nil
	}
	if hasReference {
		return 0, &errors.ValidationError{
			Msg: fmt.Sprintf("candle reference prices require reference_price_scale for symbol_id %d", symbolID),
		}
	}
	return 0, nil
}

func (s *MarketDataService) requireMarketDataVolumeScale(symbolID uint32, label string) (int, error) {
	if s.catalogs != nil {
		if scale, ok := s.catalogs.MarketDataVolumeScaleForSymbolID(symbolID); ok {
			return scale, nil
		}
	}
	return 0, &errors.ValidationError{
		Msg: fmt.Sprintf("%s requires a hydrated market_data_volume_scale for symbol_id %d", label, symbolID),
	}
}

func (s *MarketDataService) requireQuantityScale(symbolID uint32, label string) (int, error) {
	if s.catalogs != nil {
		if scale, ok := s.catalogs.BaseQuantityScaleForSymbolID(symbolID); ok {
			return scale, nil
		}
	}
	return 0, &errors.ValidationError{
		Msg: fmt.Sprintf("%s requires a hydrated catalog quantity scale for symbol_id %d", label, symbolID),
	}
}

// resolveCandleChannelTimeframe reads a candle timeframe as
// resolveCandleTimeframe does and names it as its live channel does (MIN_1
// reads as 1m).
func resolveCandleChannelTimeframe(timeframe string) (string, error) {
	value, err := resolveCandleTimeframe(timeframe)
	if err != nil {
		return "", err
	}
	return candleChannelLabels[value], nil
}
