package services

import (
	"context"
	"fmt"
	"strings"
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

// Get reads a snapshot of symbol with depth levels per side: one of the
// contract's depth tiers, 1, 5, 10, 20, 50, 100, 200, 500, 1000 or 5000
// (codecs.OrderbookDepthTiers), or 0 for the default of 50. Any other depth is
// an *errors.ValidationError before any request. The result's Depth is the
// tier the service served. Order snapshots by
// (BookEpoch, BookSeq) with orderbook.CompareSnapshots, and never apply live
// deltas to a snapshot: it is a frozen view of the book.
func (s *OrderbookService) Get(ctx context.Context, symbol string, depth int) (models.OrderbookData, error) {
	reported, err := codecs.OrderbookSnapshotDepth(depth)
	if err != nil {
		return models.OrderbookData{}, err
	}
	// Levels are decoded with the pair's catalog quantity scale.
	if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
		return models.OrderbookData{}, err
	}
	symbolID, err := ResolveSymbolID(s.catalogs, &symbol, nil, "orderbook.get")
	if err != nil {
		return models.OrderbookData{}, err
	}
	tier, err := codecs.DepthTierForLevels(reported)
	if err != nil {
		return models.OrderbookData{}, err
	}
	req := &orderbookv1.GetOrderBookRequest{SymbolId: symbolID, Depth: tier}
	scale, ok := s.catalogs.BaseQuantityScaleForSymbol(symbol)
	if !ok {
		return models.OrderbookData{}, &errors.ValidationError{Msg: fmt.Sprintf("orderbook decoding requires a hydrated catalog quantity scale for %q; leave Config.SkipCatalogHydration unset and call Client.WaitForCatalogs(ctx), or hydrate the spot catalog with Client.Catalogs.HydrateSpotConfig", symbol)}
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) (models.OrderbookData, error) {
		return decode.OrderbookFromProto(msg, symbol, reported, scale)
	})
}

// CreateSubscriptionOptions configures a managed orderbook subscription.
type CreateSubscriptionOptions struct {
	// Symbol or SymbolID selects the pair; with SymbolID alone, the symbol and
	// quantity scale come from the spot catalog.
	Symbol   string
	SymbolID *uint32
	Depth    int
	// Bucket aggregates levels client-side: "" for none, or a positive price
	// with at most 9 decimal places, such as "0.01". Any other bucket is an
	// *errors.ValidationError before connecting.
	Bucket  string
	OnEvent func(models.OrderbookData)
	// OnSequenceGap is called once when a delta does not continue the book:
	// the book then waits for the stream's next reset, published about every
	// 30 seconds, and its views read Available false until then.
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
//
// Each view's Ts is the server timestamp of the latest snapshot or delta
// applied to the book, nil when that one carried none.
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
	initialBucketTicks, err := orderbook.ParseBucketTicks(opts.Bucket)
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
			"orderbook subscription for %q needs the spot catalog for its symbol_id and quantity scale: leave Config.SkipCatalogHydration unset and call Client.WaitForCatalogs(ctx) first; a SymbolID does not replace the catalog",
			symbol,
		)}
	}
	if opts.SymbolID != nil {
		// The same rule as every call that takes both selectors.
		if _, err := ResolveSymbolID(s.catalogs, &symbol, opts.SymbolID, "orderbook.create_subscription"); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(symbol) == "" {
		// Selected by SymbolID alone: the catalog names the pair, whose
		// quantity scale decodes the levels.
		canonical := s.catalogs.SymbolForSymbolID(*resolvedSymbolID)
		if canonical == nil {
			return nil, &errors.ValidationError{Msg: fmt.Sprintf(
				"orderbook subscription for symbol_id %d needs that pair in the spot catalog: leave Config.SkipCatalogHydration unset and call Client.WaitForCatalogs(ctx), or hydrate the spot catalog with Client.Catalogs.HydrateSpotConfig",
				*resolvedSymbolID,
			)}
		}
		symbol = *canonical
	}
	channel := fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:%d:proto", streamDepth, *resolvedSymbolID)

	quantityScale, ok := s.catalogs.BaseQuantityScaleForSymbol(symbol)
	if !ok {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("orderbook decoding requires a hydrated catalog quantity scale for %q; leave Config.SkipCatalogHydration unset and call Client.WaitForCatalogs(ctx), or hydrate the spot catalog with Client.Catalogs.HydrateSpotConfig", symbol)}
	}
	var (
		mu             sync.Mutex
		bids           = orderbook.BookSide{}
		asks           = orderbook.BookSide{}
		currentBookSeq uint64
		// bookAvailable is the last snapshot's Available: false means the
		// book was being built or invalidated, so its merged levels are not
		// known to be complete.
		bookAvailable bool
		bucketTicks   = initialBucketTicks
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
		// awaitingReset is set when a delta did not continue the book: the
		// book is stale until the next reset or snapshot replaces it.
		awaitingReset bool
		// bookTs is the server timestamp of the latest snapshot or delta
		// applied to the book, nil when it carried none.
		bookTs *time.Time
	)

	// Views wait their turn to reach OnEvent and then Updates(): one built
	// while another is being delivered, such as by a SetBucket from OnEvent or
	// from another goroutine, is delivered after it, by whoever delivers. Views
	// are queued in the order they are built, under mu, so both surfaces see
	// one order and the last view has the latest bucket. Lock order: mu, then
	// deliverMu; the deliverer holds neither while it calls OnEvent.
	var (
		deliverMu   sync.Mutex
		delivering  bool
		undelivered []models.OrderbookData
	)
	// queueView, called with mu held, reports whether the caller delivers.
	queueView := func(data models.OrderbookData) bool {
		deliverMu.Lock()
		defer deliverMu.Unlock()
		undelivered = append(undelivered, data)
		if delivering {
			return false
		}
		delivering = true
		return true
	}
	// deliverOne hands one view to OnEvent and then Updates(), unless the
	// subscription has closed, and returns what OnEvent panicked with.
	deliverOne := func(localSub *orderbook.Subscription, view models.OrderbookData) (panicked any, didPanic bool) {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicked, didPanic = recovered, true
			}
		}()
		if opts.OnEvent != nil {
			opts.OnEvent(view)
		}
		// OnEvent may have closed the subscription; Enqueue then does nothing.
		localSub.Enqueue(view)
		return nil, false
	}
	// deliverQueued delivers the queued views in order until none is left or
	// the subscription closes, by Close or an Updates() overflow; then the
	// rest is dropped. A view OnEvent panics on is not queued, as before; the
	// views behind it are still delivered, and then the first panic goes on.
	deliverQueued := func(localSub *orderbook.Subscription) {
		var firstPanic any
		panicked := false
		for {
			closed := localSub.Closed()
			deliverMu.Lock()
			if closed || len(undelivered) == 0 {
				undelivered, delivering = nil, false
				deliverMu.Unlock()
				break
			}
			next := undelivered[0]
			undelivered = undelivered[1:]
			deliverMu.Unlock()
			if value, didPanic := deliverOne(localSub, next); didPanic && !panicked {
				firstPanic, panicked = value, true
			}
		}
		if panicked {
			panic(firstPanic)
		}
	}

	emit := func() {
		mu.Lock()
		if subscription == nil || subscription.Closed() {
			mu.Unlock()
			return
		}
		data, err := orderbook.BuildOrderbookData(symbol, depth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
		data.Available = bookAvailable && !awaitingReset
		data.Ts = copyTime(bookTs)
		lastViewUnavailable = !bookAvailable
		localSub := subscription
		deliverer := err == nil && queueView(data)
		mu.Unlock()
		if err != nil {
			// The bad level stays in the book, so no later update can render it.
			localSub.Fail(err)
			return
		}
		if deliverer {
			deliverQueued(localSub)
		}
	}

	var recheckUnavailableSnapshot func(available bool)
	// applyDelta changes the book without rendering it. It reports whether
	// the delta changed what to render, and whether it started a sequence gap.
	// After a gap the book waits for the stream's next reset, which carries
	// the whole view: a snapshot fetched now need not line up with the stream
	// either, so the SDK does not fetch one.
	applyDelta := func(delta models.OrderBookDeltaUpdate) (render, gapStarted bool) {
		mu.Lock()
		defer mu.Unlock()
		// A delta buffered behind a terminal failure must not change or render the book.
		if subscription != nil && subscription.Closed() {
			return false, false
		}
		if awaitingReset && !delta.Reset {
			return false, false
		}
		previousSeq := currentBookSeq
		newSeq, outOfSequence := orderbook.ApplyDelta(bids, asks, currentBookSeq, delta)
		currentBookSeq = newSeq
		if outOfSequence {
			if awaitingReset {
				return false, false
			}
			awaitingReset = true
			return true, true
		}
		if awaitingReset {
			if newSeq == previousSeq {
				// A stale reset: still waiting.
				return false, false
			}
			awaitingReset = false
		}
		if newSeq != previousSeq {
			// Applied: the book is now this delta's view.
			bookTs = delta.Ts
		}
		return true, false
	}
	// A live delta is one book state: one view.
	handleDelta := func(delta models.OrderBookDeltaUpdate) {
		render, gapStarted := applyDelta(delta)
		if gapStarted && opts.OnSequenceGap != nil {
			opts.OnSequenceGap()
		}
		if render {
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
			tier, err := codecs.DepthTierForLevels(streamDepth)
			if err != nil {
				return models.OrderbookData{}, err
			}
			req := &orderbookv1.GetOrderBookRequest{SymbolId: *resolvedSymbolID, Depth: tier}
			return UnaryPublicDecoded(fetchCtx, s.transport, s.client().GetOrderBook, req, func(msg *orderbookv1.GetOrderBookResponse) (models.OrderbookData, error) {
				// Only a snapshot of the stream's own tier is a base for its
				// deltas: check it before it touches the book.
				if err := decode.ServedDepthTier("GetOrderBook", tier, msg.GetDepth()); err != nil {
					return models.OrderbookData{}, err
				}
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
				currentBookSeq = msg.GetBookSeq()
				bookAvailable = msg.GetAvailable()
				bookTs = nil
				if ts := msg.GetTs(); ts != nil {
					value := ts.AsTime()
					bookTs = &value
				}
				// A new snapshot is a new starting view: deltas continue from it.
				awaitingReset = false
				data, err := orderbook.BuildOrderbookData(symbol, depth, currentBookSeq, bids, asks, bucketTicks, quantityScale)
				data.Available = bookAvailable
				data.Ts = copyTime(bookTs)
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
			gap := false
			for _, delta := range buffered {
				_, gapStarted := applyDelta(delta)
				gap = gap || gapStarted
			}
			if gap && opts.OnSequenceGap != nil {
				opts.OnSequenceGap()
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
	if err := subscription.SetBucket(opts.Bucket); err != nil {
		subscription.Close()
		return nil, err
	}
	if err := stream.Start(ctx); err != nil {
		subscription.Close()
		return nil, err
	}
	return subscription, nil
}

// copyTime gives each view its own timestamp, so a caller changing one
// cannot change the book's or another view's.
func copyTime(ts *time.Time) *time.Time {
	if ts == nil {
		return nil
	}
	value := *ts
	return &value
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
