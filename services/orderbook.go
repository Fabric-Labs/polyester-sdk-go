package services

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type OrderbookService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	realtime  RealtimeClient
}

func NewOrderbookService(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *OrderbookService {
	if cats == nil {
		cats = catalogs.NewManager()
	}
	return &OrderbookService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *OrderbookService) client() orderbookv1connect.OrderbookServiceClient {
	return orderbookv1connect.NewOrderbookServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *OrderbookService) Get(ctx context.Context, symbol string, depth int) (models.OrderbookData, error) {
	name := codecs.DepthToConnectEnum(depth)
	v, ok := orderbookv1.Depth_value[name]
	if !ok {
		return models.OrderbookData{}, &errors.ValidationError{Msg: "unsupported orderbook depth"}
	}
	req := &orderbookv1.GetOrderBookRequest{Symbol: symbol, Depth: orderbookv1.Depth(v)}
	scale := s.catalogs.BaseQuantityScaleForSymbol(symbol)
	return UnaryPublic(ctx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) models.OrderbookData {
		return decode.OrderbookFromProto(msg, symbol, depth, scale)
	})
}

// CreateSubscriptionOptions configures a managed orderbook subscription.
type CreateSubscriptionOptions struct {
	Symbol   string
	SymbolID *uint32
	Depth    int
	Bucket   string
	OnEvent  func(models.OrderbookData)
	// OnSequenceGap is called when a book sequence gap is detected (before snapshot refresh).
	OnSequenceGap func()
	// OnReconnect is called after a websocket disconnect, before snapshot rebuild.
	OnReconnect func()
	// OnSnapshotRefresh is called after a successful snapshot rebuild.
	OnSnapshotRefresh func()
}

// CreateSubscription starts snapshot-then-stream orderbook merging.
func (s *OrderbookService) CreateSubscription(ctx context.Context, opts CreateSubscriptionOptions) (*orderbook.Subscription, error) {
	if err := requireRealtime(s.realtime); err != nil {
		return nil, err
	}
	symbol := opts.Symbol
	depth := opts.Depth
	if depth <= 0 {
		depth = 50
	}
	wsDepth := int(math.Min(500, math.Max(1, float64(depth))))
	resolvedSymbolID := opts.SymbolID
	if resolvedSymbolID == nil {
		resolvedSymbolID = s.catalogs.SymbolIDForSymbol(symbol)
	}
	if resolvedSymbolID == nil || *resolvedSymbolID == 0 {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("symbol_id is required for orderbook subscriptions (%q)", symbol)}
	}
	channel := fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:%d:proto", wsDepth, *resolvedSymbolID)

	var (
		mu             sync.Mutex
		bids           = orderbook.BookSide{}
		asks           = orderbook.BookSide{}
		currentBookSeq int
		bucketTicks    = orderbook.ParseBucketTicks(opts.Bucket)
		quantityScale  = s.catalogs.BaseQuantityScaleForSymbol(symbol)
		subscription   *orderbook.Subscription
		stream         *realtime.SnapshotThenStream[models.OrderbookData, models.OrderBookDeltaUpdate]
	)

	emit := func() {
		mu.Lock()
		defer mu.Unlock()
		if subscription == nil {
			return
		}
		data := orderbook.BuildOrderbookData(symbol, wsDepth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
		if opts.OnEvent != nil {
			opts.OnEvent(data)
		}
		subscription.Enqueue(data)
	}

	handleDelta := func(delta models.OrderBookDeltaUpdate) {
		mu.Lock()
		newSeq, needsRefresh := orderbook.ApplyDelta(bids, asks, currentBookSeq, delta)
		currentBookSeq = newSeq
		localStream := stream
		mu.Unlock()
		if needsRefresh && localStream != nil {
			if opts.OnSequenceGap != nil {
				opts.OnSequenceGap()
			}
			_ = localStream.RefreshSnapshot(ctx)
			return
		}
		emit()
	}

	stream = realtime.NewSnapshotThenStream(realtime.SnapshotThenStreamConfig[models.OrderbookData, models.OrderBookDeltaUpdate]{
		Client:  s.realtime,
		Channel: channel,
		Decode:  decode.OrderbookDeltaFromBytes,
		OnReconnect: opts.OnReconnect,
		OnSnapshotRefresh: opts.OnSnapshotRefresh,
		FetchSnapshot: func(fetchCtx context.Context) (models.OrderbookData, error) {
			name := codecs.DepthToConnectEnum(wsDepth)
			v, ok := orderbookv1.Depth_value[name]
			if !ok {
				return models.OrderbookData{}, &errors.ValidationError{Msg: "unsupported orderbook depth"}
			}
			req := &orderbookv1.GetOrderBookRequest{Symbol: symbol, Depth: orderbookv1.Depth(v)}
			return UnaryPublic(fetchCtx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) models.OrderbookData {
				mu.Lock()
				bids = orderbook.LevelsFromProtoLevels(msg.GetBids())
				asks = orderbook.LevelsFromProtoLevels(msg.GetAsks())
				currentBookSeq = int(msg.GetBookSeq())
				mu.Unlock()
				return orderbook.BuildOrderbookData(symbol, wsDepth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
			})
		},
		ReadPublication: func(delta models.OrderBookDeltaUpdate) []models.OrderBookDeltaUpdate {
			return []models.OrderBookDeltaUpdate{delta}
		},
		ApplySnapshot: func(snapshot models.OrderbookData, buffered []models.OrderBookDeltaUpdate) {
			_ = snapshot
			for _, delta := range buffered {
				handleDelta(delta)
			}
			emit()
		},
		ApplyLivePublications: func(deltas []models.OrderBookDeltaUpdate) {
			for _, delta := range deltas {
				handleDelta(delta)
			}
		},
		MaxBuffered: 200,
	})

	subscription = orderbook.NewSubscription(stream, emit)
	subscription.SetBucket(opts.Bucket)
	if err := stream.Start(ctx); err != nil {
		subscription.Close()
		return nil, err
	}
	return subscription, nil
}

func (s *OrderbookService) SubscribeDeltas(ctx context.Context, symbolID uint32, depth int) (*realtime.Subscription[models.OrderBookDeltaUpdate], error) {
	wsDepth := int(math.Min(500, math.Max(1, float64(depth))))
	channel := fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:%d:proto", wsDepth, symbolID)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.OrderbookDeltaFromBytes)
}

func (s *OrderbookService) Subscribe(ctx context.Context, symbol string, symbolID *uint32, depth int) (*orderbook.Subscription, error) {
	return s.CreateSubscription(ctx, CreateSubscriptionOptions{Symbol: symbol, SymbolID: symbolID, Depth: depth})
}
