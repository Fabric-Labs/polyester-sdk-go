package services

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// A snapshot that came back unavailable is requested again after these delays:
// the first delay doubles up to the limit, for as long as the stream runs.
const (
	unavailableSnapshotInitialDelay = time.Second
	unavailableSnapshotMaxDelay     = 8 * time.Second
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

// Get reads a snapshot of symbol. depth is a snapshot depth (1, 5, 10, 20, 50,
// 100, 200, 500 or 1000); any other depth from 1 to 1000 requests the smallest
// one covering it, and 0 means 50. The result's Depth is the depth requested.
// A depth below 0 or above 1000 is an *errors.ValidationError before any request.
func (s *OrderbookService) Get(ctx context.Context, symbol string, depth int) (models.OrderbookData, error) {
	depth, err := codecs.OrderbookSnapshotDepth(depth)
	if err != nil {
		return models.OrderbookData{}, err
	}
	name := codecs.DepthToConnectEnum(depth)
	v, ok := orderbookv1.Depth_value[name]
	if !ok {
		return models.OrderbookData{}, &errors.ValidationError{Msg: "unsupported orderbook depth"}
	}
	// Levels are decoded with the pair's catalog quantity scale.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return models.OrderbookData{}, err
	}
	symbolID, err := ResolveSymbolID(s.catalogs, &symbol, nil, "orderbook.get")
	if err != nil {
		return models.OrderbookData{}, err
	}
	req := &orderbookv1.GetOrderBookRequest{SymbolId: symbolID, Depth: orderbookv1.Depth(v)}
	scale, ok := s.catalogs.BaseQuantityScaleForSymbol(symbol)
	if !ok {
		return models.OrderbookData{}, &errors.ValidationError{Msg: fmt.Sprintf("orderbook decoding requires a hydrated catalog quantity scale for %q; set Config.HydrateCatalogs: true and call Client.WaitForCatalogs(ctx), or hydrate the spot catalog with Client.Catalogs.HydrateSpotConfig", symbol)}
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) (models.OrderbookData, error) {
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
	// OnError is called for transport, decode, snapshot, terminal queue and book render errors.
	OnError func(error)
}

// CreateSubscription starts snapshot-then-stream orderbook merging.
//
// opts.Depth is any level count from 1 to the deepest published stream tier, and
// 0 means 50. The book rides the smallest tier covering it, snapshot included,
// and each view is trimmed to it; a depth no tier covers is a ValidationError
// before connecting.
func (s *OrderbookService) CreateSubscription(ctx context.Context, opts CreateSubscriptionOptions) (*orderbook.Subscription, error) {
	if err := requireRealtime(s.realtime); err != nil {
		return nil, err
	}
	symbol := opts.Symbol
	depth := opts.Depth
	if depth == 0 {
		depth = 50
	}
	streamDepth, err := codecs.OrderbookStreamTier(depth)
	if err != nil {
		return nil, err
	}
	resolvedSymbolID := opts.SymbolID
	if s.catalogs.Hydration() != nil {
		// Levels are decoded with the pair's catalog quantity scale.
		if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
			return nil, err
		}
		if resolvedSymbolID == nil {
			id, err := ResolveSymbolID(s.catalogs, &symbol, nil, "orderbook.create_subscription")
			if err != nil {
				return nil, err
			}
			resolvedSymbolID = &id
		}
	}
	if resolvedSymbolID == nil {
		resolvedSymbolID = s.catalogs.SymbolIDForSymbol(symbol)
	}
	if resolvedSymbolID == nil || *resolvedSymbolID == 0 {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf(
			"orderbook subscription for %q needs the spot catalog for its symbol_id and quantity scale: set Config.HydrateCatalogs: true and call Client.WaitForCatalogs(ctx) first; a SymbolID does not replace the catalog",
			symbol,
		)}
	}
	if opts.SymbolID != nil {
		// The same rule as every call that takes both selectors.
		if _, err := ResolveSymbolID(s.catalogs, &symbol, opts.SymbolID, "orderbook.create_subscription"); err != nil {
			return nil, err
		}
	}
	channel := fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:%d:proto", streamDepth, *resolvedSymbolID)

	quantityScale, ok := s.catalogs.BaseQuantityScaleForSymbol(symbol)
	if !ok {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("orderbook decoding requires a hydrated catalog quantity scale for %q; set Config.HydrateCatalogs: true and call Client.WaitForCatalogs(ctx), or hydrate the spot catalog with Client.Catalogs.HydrateSpotConfig", symbol)}
	}
	var (
		mu             sync.Mutex
		bids           = orderbook.BookSide{}
		asks           = orderbook.BookSide{}
		currentBookSeq int
		// bookAvailable is the last snapshot's Available: false means the
		// book was being built or invalidated, so its merged levels are not
		// known to be complete.
		bookAvailable bool
		bucketTicks   = orderbook.ParseBucketTicks(opts.Bucket)
		subscription  *orderbook.Subscription
		stream        *realtime.SnapshotThenStream[models.OrderbookData, models.OrderBookDeltaUpdate]
		// lastViewUnavailable, recheckPending and recheckDelay pace requests for
		// a snapshot that came back unavailable.
		lastViewUnavailable bool
		recheckPending      bool
		recheckDelay        = unavailableSnapshotInitialDelay
		// recheckGeneration invalidates a scheduled request once a newer
		// snapshot is available.
		recheckGeneration uint64
	)

	emit := func() {
		mu.Lock()
		if subscription == nil || subscription.Closed() {
			mu.Unlock()
			return
		}
		data, err := orderbook.BuildOrderbookData(symbol, depth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
		data.Available = bookAvailable
		lastViewUnavailable = !bookAvailable
		localSub := subscription
		mu.Unlock()
		if err != nil {
			// The bad level stays in the book, so no later update can render it.
			localSub.Fail(err)
			return
		}
		if opts.OnEvent != nil {
			opts.OnEvent(data)
		}
		localSub.Enqueue(data)
	}

	var recheckUnavailableSnapshot func(available bool)
	// applyDelta changes the book without rendering it. It reports false when
	// the delta changed nothing to render: the subscription ended, or a
	// sequence gap requested a refresh.
	applyDelta := func(delta models.OrderBookDeltaUpdate) bool {
		mu.Lock()
		// A delta buffered behind a terminal failure must not change or render the book.
		if subscription != nil && subscription.Closed() {
			mu.Unlock()
			return false
		}
		newSeq, needsRefresh := orderbook.ApplyDelta(bids, asks, currentBookSeq, delta)
		currentBookSeq = newSeq
		localStream := stream
		mu.Unlock()
		if needsRefresh && localStream != nil {
			if opts.OnSequenceGap != nil {
				opts.OnSequenceGap()
			}
			localStream.RequestSnapshotRefresh(ctx)
			return false
		}
		return true
	}
	// A live delta is one book state: one view.
	handleDelta := func(delta models.OrderBookDeltaUpdate) {
		if applyDelta(delta) {
			emit()
		}
	}

	// An unavailable book's levels are unknown, not empty: ask again later, with a
	// paced delay, until the snapshot is available or the stream stops.
	recheckUnavailableSnapshot = func(available bool) {
		mu.Lock()
		if available {
			recheckDelay = unavailableSnapshotInitialDelay
			recheckPending = false
			recheckGeneration++
			mu.Unlock()
			return
		}
		if recheckPending || stream == nil {
			mu.Unlock()
			return
		}
		recheckPending = true
		delay := recheckDelay
		recheckDelay = min(recheckDelay*2, unavailableSnapshotMaxDelay)
		localStream := stream
		generation := recheckGeneration
		mu.Unlock()
		go func() {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-localStream.Done():
				return
			case <-ctx.Done():
				return
			}
			mu.Lock()
			if generation != recheckGeneration {
				mu.Unlock()
				return
			}
			recheckPending = false
			mu.Unlock()
			localStream.RequestSnapshotRefresh(ctx)
		}()
	}

	stream = realtime.NewSnapshotThenStream(realtime.SnapshotThenStreamConfig[models.OrderbookData, models.OrderBookDeltaUpdate]{
		Client:            s.realtime,
		Channel:           channel,
		Decode:            decode.OrderbookDeltaFromBytes,
		OnReconnect:       opts.OnReconnect,
		OnSnapshotRefresh: opts.OnSnapshotRefresh,
		OnError:           opts.OnError,
		FetchSnapshot: func(fetchCtx context.Context) (models.OrderbookData, error) {
			// At the tier, not the requested depth: deltas diff the tier's whole
			// view, and a level promoted into a trimmed view must already be known.
			name := codecs.DepthToConnectEnum(streamDepth)
			v, ok := orderbookv1.Depth_value[name]
			if !ok {
				return models.OrderbookData{}, &errors.ValidationError{Msg: "unsupported orderbook depth"}
			}
			req := &orderbookv1.GetOrderBookRequest{SymbolId: *resolvedSymbolID, Depth: orderbookv1.Depth(v)}
			return UnaryPublicDecoded(fetchCtx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) (models.OrderbookData, error) {
				nextBids, err := orderbook.LevelsFromProtoLevels(msg.GetBids())
				if err != nil {
					return models.OrderbookData{}, err
				}
				nextAsks, err := orderbook.LevelsFromProtoLevels(msg.GetAsks())
				if err != nil {
					return models.OrderbookData{}, err
				}
				mu.Lock()
				bids = nextBids
				asks = nextAsks
				currentBookSeq = int(msg.GetBookSeq())
				bookAvailable = msg.GetAvailable()
				data, err := orderbook.BuildOrderbookData(symbol, depth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
				data.Available = bookAvailable
				mu.Unlock()
				return data, err
			})
		},
		ReadPublication: func(delta models.OrderBookDeltaUpdate) []models.OrderBookDeltaUpdate {
			mu.Lock()
			localSub := subscription
			mu.Unlock()
			if localSub != nil {
				localSub.NotePublication()
			}
			return []models.OrderBookDeltaUpdate{delta}
		},
		ApplySnapshot: func(snapshot models.OrderbookData, buffered []models.OrderBookDeltaUpdate) {
			_ = snapshot
			// The snapshot and the deltas buffered behind it are one book state,
			// so they yield one view: a view per delta would queue stale copies.
			for _, delta := range buffered {
				applyDelta(delta)
			}
			mu.Lock()
			available := bookAvailable
			repeated := !available && lastViewUnavailable && len(buffered) == 0
			mu.Unlock()
			if !repeated {
				emit()
			}
			recheckUnavailableSnapshot(available)
		},
		ApplyLivePublications: func(deltas []models.OrderBookDeltaUpdate) {
			for _, delta := range deltas {
				handleDelta(delta)
			}
		},
		MaxBuffered: 200,
	})

	subscription = orderbook.NewSubscription(stream, emit, func(ticks int) {
		mu.Lock()
		bucketTicks = ticks
		mu.Unlock()
	})
	subscription.SetOnError(opts.OnError)
	subscription.SetBucket(opts.Bucket)
	if err := stream.Start(ctx); err != nil {
		subscription.Close()
		return nil, err
	}
	return subscription, nil
}

// SubscribeDeltas delivers the raw deltas of one published stream depth,
// codecs.OrderbookStreamDepths; any other depth is a ValidationError.
func (s *OrderbookService) SubscribeDeltas(ctx context.Context, symbolID uint32, depth int) (*realtime.Subscription[models.OrderBookDeltaUpdate], error) {
	streamDepth, err := codecs.RequirePublishedStreamDepth(depth)
	if err != nil {
		return nil, err
	}
	channel := fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:%d:proto", streamDepth, symbolID)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.OrderbookDeltaFromBytes)
}

func (s *OrderbookService) Subscribe(ctx context.Context, symbol string, symbolID *uint32, depth int) (*orderbook.Subscription, error) {
	return s.CreateSubscription(ctx, CreateSubscriptionOptions{Symbol: symbol, SymbolID: symbolID, Depth: depth})
}
