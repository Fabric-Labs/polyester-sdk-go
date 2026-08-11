package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var timeframeAliases = map[string]string{"1s": "SEC_1", "1m": "MIN_1", "5m": "MIN_5", "15m": "MIN_15", "30m": "MIN_30", "1h": "HOUR_1", "4h": "HOUR_4", "12h": "HOUR_12", "1d": "DAY_1", "1w": "WEEK_1", "1mo": "MONTH_1"}

// Live Centrifugo channels use human labels (`1m`), not REST enum names (`MIN_1`).
var channelTimeframeAliases = func() map[string]string {
	out := make(map[string]string, len(timeframeAliases)*4)
	for label, enumName := range timeframeAliases {
		out[label] = label
		out[enumName] = label
		out[strings.ToLower(enumName)] = label
		out[strings.ToLower(strings.ReplaceAll(enumName, "_", ""))] = label
	}
	return out
}()

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
	return UnaryPublic(ctx, s.transport, s.client().GetSpotConfig, &marketdatav1.GetSpotConfigRequest{}, decode.SpotConfigFromProto)
}

func (s *MarketDataService) GetTrades(ctx context.Context, symbol *string, symbolID *uint32, limit int, pageToken *string) (models.MarketTradesResult, error) {
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
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetTrades, req, func(msg *marketdatav1.GetTradesResponse) (models.MarketTradesResult, error) {
		return decode.MarketTradesFromProtoChecked(msg, scale)
	})
}

// GetCandles returns OHLCV candles newest-first. When includeIncomplete is true,
// the open candle, if present, is prepended.
func (s *MarketDataService) GetCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool) (models.CandlesResult, error) {
	req, scale, err := s.buildCandlesRequest(symbol, symbolID, timeframe, limit, start, end, includeIncomplete, nil, false)
	if err != nil {
		return models.CandlesResult{}, err
	}
	return UnaryPublic(ctx, s.transport, s.client().GetCandles, req, func(msg *marketdatav1.GetCandlesResponse) models.CandlesResult {
		return decode.CandlesFromProto(msg, scale)
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
func (s *MarketDataService) GetCandlesColumns(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken *string) (models.CandlesResult, error) {
	base, scale, err := s.buildCandlesRequest(symbol, symbolID, timeframe, limit, start, end, includeIncomplete, pageToken, false)
	if err != nil {
		return models.CandlesResult{}, err
	}
	req := &marketdatav1.GetCandlesColumnsRequest{
		SymbolId: base.SymbolId, Timeframe: base.Timeframe, Limit: base.Limit,
		StartTime: base.StartTime, EndTime: base.EndTime, IncludeIncomplete: base.IncludeIncomplete,
		IncludeReference: base.IncludeReference, PageToken: base.PageToken,
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetCandlesColumns, req, func(msg *marketdatav1.GetCandlesColumnsResponse) (models.CandlesResult, error) {
		return decode.CandlesColumnsFromProto(msg, scale)
	})
}

func (s *MarketDataService) buildCandlesRequest(symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool, pageToken *string, includeReference bool) (*marketdatav1.GetCandlesRequest, int, error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "get_candles")
	if err != nil {
		return nil, 0, err
	}
	name := timeframeAliases[timeframe]
	if name == "" {
		name = timeframe
	}
	if v, ok := marketdatav1.Timeframe_value[name]; ok {
		parsedLimit, err := PaginationLimit(limit, "limit")
		if err != nil {
			return nil, 0, err
		}
		req := &marketdatav1.GetCandlesRequest{SymbolId: resolved, Timeframe: marketdatav1.Timeframe(v), Limit: parsedLimit, IncludeIncomplete: includeIncomplete, IncludeReference: includeReference}
		if start != nil {
			req.StartTime = timestamppb.New(*start)
		}
		if end != nil {
			req.EndTime = timestamppb.New(*end)
		}
		if pageToken != nil {
			req.PageToken = *pageToken
		}
		scale, err := s.requireQuantityScale(resolved, "candle volume")
		if err != nil {
			return nil, 0, err
		}
		return req, scale, nil
	}
	return nil, 0, &errors.ValidationError{Msg: "Unknown candle timeframe; use aliases like '1m', '1h', '1d'"}
}

func (s *MarketDataService) SubscribeTrades(ctx context.Context, symbol *string, symbolID *uint32) (*realtime.Subscription[models.MarketTrade], error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_trades")
	if err != nil {
		return nil, err
	}
	scale, err := s.requireQuantityScale(resolved, "subscribe_trades")
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:market:trades:%d:proto", resolved)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.MarketTradeFromBytes(scale))
}

func (s *MarketDataService) SubscribeCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string) (*realtime.Subscription[models.Candle], error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_candles")
	if err != nil {
		return nil, err
	}
	channelTimeframe, err := resolveCandleChannelTimeframe(timeframe)
	if err != nil {
		return nil, err
	}
	volumeScale, err := s.requireQuantityScale(resolved, "candle volume")
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:market:candles:%s:%d:proto", channelTimeframe, resolved)
	decodeFn := decode.CandlePointFromBytes(resolved, channelTimeframe, volumeScale)
	return SubscribePublicProto(ctx, s.realtime, channel, decodeFn)
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

// resolveCandleChannelTimeframe normalizes user timeframe aliases to the live channel label.
func resolveCandleChannelTimeframe(timeframe string) (string, error) {
	if label, ok := channelTimeframeAliases[timeframe]; ok {
		return label, nil
	}
	if label, ok := channelTimeframeAliases[strings.ToUpper(timeframe)]; ok {
		return label, nil
	}
	compact := strings.ToLower(strings.ReplaceAll(timeframe, "_", ""))
	if label, ok := channelTimeframeAliases[compact]; ok {
		return label, nil
	}
	return "", &errors.ValidationError{Msg: "Unknown candle timeframe; use aliases like '1m', '1h', '1d'"}
}
