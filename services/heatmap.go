package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	heatmapv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HeatmapService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	realtime  RealtimeClient
}

func NewHeatmapService(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *HeatmapService {
	return &HeatmapService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *HeatmapService) client() marketdatav1connect.HeatmapServiceClient {
	return marketdatav1connect.NewHeatmapServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *HeatmapService) Get(ctx context.Context, symbol *string, symbolID *uint32, interval string, depth, limit int, quantityMode string, fromTsSec *int64, start, end *time.Time) (models.ApiData, error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "heatmap.get")
	if err != nil {
		return models.ApiData{}, err
	}
	intervalName := codecs.IntervalAliases[interval]
	if intervalName == "" {
		intervalName = interval
	}
	intervalEnum, ok := heatmapv1.HeatmapInterval_value[intervalName]
	if !ok {
		return models.ApiData{}, &errors.ValidationError{Msg: "Unknown heatmap interval"}
	}
	depthName := codecs.DepthToProtoName(depth)
	depthEnum, ok := heatmapv1.HeatmapDepth_value[depthName]
	if !ok {
		return models.ApiData{}, &errors.ValidationError{Msg: "Unsupported heatmap depth"}
	}
	qtyName := codecs.QtyModeAliases[quantityMode]
	if qtyName == "" {
		qtyName = quantityMode
	}
	qtyEnum, ok := heatmapv1.HeatmapQuantityMode_value[qtyName]
	if !ok {
		return models.ApiData{}, &errors.ValidationError{Msg: "quantity_mode must be 'close' or 'peak'"}
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &heatmapv1.GetOrderbookHeatmapRequest{SymbolId: resolved, Interval: heatmapv1.HeatmapInterval(intervalEnum), Depth: heatmapv1.HeatmapDepth(depthEnum), Limit: parsedLimit, QuantityMode: heatmapv1.HeatmapQuantityMode(qtyEnum)}
	tr := &heatmapv1.HeatmapTimeRange{}
	now := time.Now().UTC()
	if fromTsSec != nil {
		tr.StartTime = timestamppb.New(time.Unix(*fromTsSec, 0).UTC())
		tr.EndTime = timestamppb.New(now)
	} else {
		e := now
		if end != nil {
			e = end.UTC()
		}
		st := e.Add(-5 * time.Minute)
		if start != nil {
			st = start.UTC()
		}
		tr.StartTime = timestamppb.New(st)
		tr.EndTime = timestamppb.New(e)
	}
	req.TimeRange = tr
	return UnaryPublic(ctx, s.transport, s.client().GetOrderbookHeatmap, req, func(msg *heatmapv1.GetOrderbookHeatmapResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *HeatmapService) SubscribeLive(ctx context.Context, symbol *string, symbolID *uint32, interval string) (*realtime.Subscription[models.ApiData], error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_live")
	if err != nil {
		return nil, err
	}
	intervalName := codecs.IntervalAliases[interval]
	if intervalName == "" {
		intervalName = interval
	}
	channel := fmt.Sprintf("public:spot:market:heatmap:%s:%d:proto", intervalName, resolved)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.HeatmapLiveBucketFromBytes)
}
