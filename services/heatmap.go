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

// Get reads heatmap history. Pass the returned opaque token as the optional
// final argument with unchanged filters. A continuation omits the default time
// window; any supplied range bounds are sent as given without adding current time.
// The first page runs from fromTsSec (or start, else 5 minutes before the end) to
// end, or to now when end is nil.
//
// depth is requested as codecs.HeatmapDepth gives it: a supported depth as
// itself, any other depth up to 1000 as the smallest supported depth covering
// it, and 0 as 50; the response's "depth" names the depth requested. A depth
// below 0 or above 1000 is a ValidationError before any request.
func (s *HeatmapService) Get(ctx context.Context, symbol *string, symbolID *uint32, interval string, depth, limit int, quantityMode string, fromTsSec *int64, start, end *time.Time, pageToken ...string) (models.ApiData, error) {
	if len(pageToken) > 1 {
		return models.ApiData{}, &errors.ValidationError{Msg: "at most one page token is allowed"}
	}
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
		return models.ApiData{}, &errors.ValidationError{Msg: fmt.Sprintf("unknown heatmap interval %q; use 1s, 1m, 5m or 1h", interval)}
	}
	depth, err = codecs.HeatmapDepth(depth)
	if err != nil {
		return models.ApiData{}, err
	}
	depthEnum, ok := heatmapv1.HeatmapDepth_value[fmt.Sprintf("DEPTH_%d", depth)]
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
	if len(pageToken) == 1 {
		req.PageToken = pageToken[0]
	}
	if req.PageToken != "" {
		if fromTsSec != nil || start != nil || end != nil {
			tr := &heatmapv1.HeatmapTimeRange{}
			if fromTsSec != nil {
				tr.StartTime = timestamppb.New(time.Unix(*fromTsSec, 0).UTC())
			} else if start != nil {
				tr.StartTime = timestamppb.New(start.UTC())
			}
			if end != nil {
				tr.EndTime = timestamppb.New(end.UTC())
			}
			req.TimeRange = tr
		}
	} else {
		tr := &heatmapv1.HeatmapTimeRange{}
		now := time.Now().UTC()
		e := now
		if end != nil {
			e = end.UTC()
		}
		st := e.Add(-5 * time.Minute)
		if fromTsSec != nil {
			st = time.Unix(*fromTsSec, 0).UTC()
		} else if start != nil {
			st = start.UTC()
		}
		tr.StartTime = timestamppb.New(st)
		tr.EndTime = timestamppb.New(e)
		req.TimeRange = tr
	}

	return UnaryPublic(ctx, s.transport, s.client().GetOrderbookHeatmap, req, func(msg *heatmapv1.GetOrderbookHeatmapResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

// SubscribeLive streams live heatmap buckets for interval ("1s", "1m", "5m" or
// "1h", or its enum name such as "INTERVAL_1S") from the channel the server
// publishes them on. Any other interval is a ValidationError.
func (s *HeatmapService) SubscribeLive(ctx context.Context, symbol *string, symbolID *uint32, interval string) (*realtime.Subscription[models.ApiData], error) {
	resolved, err := ResolveSymbolID(s.catalogs, symbol, symbolID, "subscribe_live")
	if err != nil {
		return nil, err
	}
	label, err := codecs.HeatmapIntervalChannelLabel(interval)
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:market:heatmap:%s:%d:proto", label, resolved)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.HeatmapLiveBucketFromBytes)
}
