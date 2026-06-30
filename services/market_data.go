package services

import (
	"context"
	"fmt"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

var timeframeAliases = map[string]string{"1s": "SEC_1", "1m": "MIN_1", "5m": "MIN_5", "15m": "MIN_15", "30m": "MIN_30", "1h": "HOUR_1", "4h": "HOUR_4", "12h": "HOUR_12", "1d": "DAY_1", "1w": "WEEK_1", "1mo": "MONTH_1"}

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
	req := &marketdatav1.GetTradesRequest{SymbolId: resolved, Limit: uint32(limit)}
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	return UnaryPublic(ctx, s.transport, s.client().GetTrades, req, decode.MarketTradesFromProto)
}

func (s *MarketDataService) GetCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string, limit int, start, end *time.Time, includeIncomplete bool) (models.CandlesResult, error) {
	req, scale, err := s.buildCandlesRequest(symbol, symbolID, timeframe, limit, start, end, includeIncomplete, nil, false)
	if err != nil {
		return models.CandlesResult{}, err
	}
	return UnaryPublic(ctx, s.transport, s.client().GetCandles, req, func(msg *marketdatav1.GetCandlesResponse) models.CandlesResult {
		return decode.CandlesFromProto(msg, scale)
	})
}

// GetCurrentCandle returns the latest candle for a symbol/timeframe.
func (s *MarketDataService) GetCurrentCandle(ctx context.Context, symbol *string, symbolID *uint32, timeframe string) (models.Candle, error) {
	result, err := s.GetCandles(ctx, symbol, symbolID, timeframe, 1, nil, nil, true)
	if err != nil {
		return models.Candle{}, err
	}
	if len(result.Candles) == 0 {
		return models.Candle{TsSec: 0}, nil
	}
	return result.Candles[len(result.Candles)-1], nil
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
	return UnaryPublic(ctx, s.transport, s.client().GetCandlesColumns, req, func(msg *marketdatav1.GetCandlesColumnsResponse) models.CandlesResult {
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
		req := &marketdatav1.GetCandlesRequest{SymbolId: resolved, Timeframe: marketdatav1.Timeframe(v), Limit: uint32(limit), IncludeIncomplete: includeIncomplete, IncludeReference: includeReference}
		if start != nil {
			req.StartTime = timestamppb.New(*start)
		}
		if end != nil {
			req.EndTime = timestamppb.New(*end)
		}
		if pageToken != nil {
			req.PageToken = *pageToken
		}
		scale := 8
		if s.catalogs != nil {
			scale = s.catalogs.BaseQuantityScaleForSymbolID(resolved)
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
	channel := fmt.Sprintf("public:spot:market:trades:%d:proto", resolved)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.MarketTradeFromBytes)
}

func (s *MarketDataService) SubscribeCandles(ctx context.Context, symbol *string, symbolID *uint32, timeframe string) (*realtime.Subscription[models.Candle], error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_candles")
	if err != nil {
		return nil, err
	}
	volumeScale := 8
	if s.catalogs != nil {
		volumeScale = s.catalogs.BaseQuantityScaleForSymbolID(resolved)
	}
	channel := fmt.Sprintf("public:spot:market:candles:%s:%d:proto", timeframe, resolved)
	decodeFn := decode.CandlePointFromBytes(resolved, timeframe, volumeScale)
	return SubscribePublicProto(ctx, s.realtime, channel, decodeFn)
}
