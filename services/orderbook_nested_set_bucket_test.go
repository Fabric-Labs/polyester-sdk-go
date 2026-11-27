package services

import (
	"context"
	stderrors "errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// A bid of 71_600_000 ticks as it reads ungrouped, by "0.01" and by "0.001".
const (
	nestedRaw   = 71_600_000
	nestedCent  = 70_000_000
	nestedMilli = 71_000_000
)

// nestedBucketSubscription opens a depth-20 subscription whose OnEvent
// records each view's best bid and then runs onView. It returns the
// subscription and the bids OnEvent has seen so far.
func nestedBucketSubscription(t *testing.T, onView func(*orderbook.Subscription, models.OrderbookData)) (*orderbook.Subscription, func() []int64) {
	t.Helper()
	sub, seen, _ := nestedBucketStream(t, onView, func(*CreateSubscriptionOptions) {})
	return sub, seen
}

// nestedBucketStream is nestedBucketSubscription with configure applied to
// the options, and a function that publishes a frame on its channel.
func nestedBucketStream(t *testing.T, onView func(*orderbook.Subscription, models.OrderbookData), configure func(*CreateSubscriptionOptions)) (*orderbook.Subscription, func() []int64, func([]byte)) {
	t.Helper()
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	done := make(chan struct{})
	frames := make(chan []byte)
	ws := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
			if err = conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		for {
			select {
			case frame := <-frames:
				if err = conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}))
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{
			SymbolId: 1, BookSeq: 1, Available: true, Depth: requestedTier(r),
			Bids: []*ob.PriceLevel{{PriceTicks: nestedRaw, QtyScaled: 1}},
			Asks: []*ob.PriceLevel{{PriceTicks: 71_700_000, QtyScaled: 4}},
		})
		_, _ = w.Write(data)
	}))
	t.Cleanup(func() { close(done); ws.Close(); httpSrv.Close() })
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "DOGE-USDT", "symbol_id": 1, "base_quantity_scale": 8},
	}}); err != nil {
		t.Fatal(err)
	}
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	svc := NewOrderbookService(transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil), cats, rt)

	var mu sync.Mutex
	var sub *orderbook.Subscription
	var called []int64
	onEvent := func(view models.OrderbookData) {
		mu.Lock()
		called = append(called, view.Bids[0].Price.Ticks())
		current := sub
		mu.Unlock()
		// The snapshot's view arrives during CreateSubscription, before sub is set.
		if current != nil {
			onView(current, view)
		}
	}
	opts := CreateSubscriptionOptions{Symbol: "DOGE-USDT", Depth: 20, OnEvent: onEvent}
	configure(&opts)
	created, err := svc.CreateSubscription(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(created.Close)
	mu.Lock()
	sub = created
	mu.Unlock()
	return created, func() []int64 {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(called)
		}, func(frame []byte) {
			select {
			case frames <- frame:
			case <-time.After(10 * time.Second):
				t.Error("the test server did not take the frame")
			}
		}
}

// liveDelta changes the best bid's quantity at sequence 2.
func liveDelta() []byte {
	return bookTicketFrame(&ob.OrderBookDelta{
		SymbolId: 1, BookSeqStart: 1, BookSeqEnd: 2,
		Bids: []*ob.PriceLevel{{PriceTicks: nestedRaw, QtyScaled: 2}},
	})
}

// gapDelta does not continue sequence 2: the stream worker reports a gap.
func gapDelta() []byte {
	return bookTicketFrame(&ob.OrderBookDelta{
		SymbolId: 1, BookSeqStart: 5, BookSeqEnd: 6,
		Bids: []*ob.PriceLevel{{PriceTicks: nestedRaw, QtyScaled: 3}},
	})
}

// gate blocks an OnEvent until it is opened, and is opened at the end of the
// test whatever happens, so a failure cannot leave a goroutine blocked.
type gate struct {
	entered, opened chan struct{}
	enter, open     sync.Once
}

func newGate(t *testing.T) *gate {
	g := &gate{entered: make(chan struct{}), opened: make(chan struct{})}
	t.Cleanup(g.release)
	return g
}

// wait, from the OnEvent, reports that it is blocked and blocks.
func (g *gate) wait() {
	g.enter.Do(func() { close(g.entered) })
	<-g.opened
}

func (g *gate) release() { g.open.Do(func() { close(g.opened) }) }

func receive(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func reportedError(t *testing.T, reported <-chan error) error {
	t.Helper()
	select {
	case err := <-reported:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("OnError was not called")
		return nil
	}
}

// A live delta's view, queued behind a SetBucket's blocked OnEvent and
// delivered by that SetBucket caller, panics in OnEvent. As when the stream
// worker delivers it itself, the panic fails the subscription closed with an
// OnError, and does not escape through the unrelated SetBucket call.
func TestOrderbookALiveViewPanicDeliveredByASetBucketCallerFailsTheStream(t *testing.T) {
	blocked := newGate(t)
	gap := make(chan struct{}, 1)
	reported := make(chan error, 1)
	sub, _, push := nestedBucketStream(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		switch {
		case view.BookSeq == "1" && view.Bids[0].Price.Ticks() == nestedCent:
			blocked.wait()
		case view.BookSeq == "2":
			panic("live view panicked")
		}
	}, func(opts *CreateSubscriptionOptions) {
		opts.OnError = func(err error) { reported <- err }
		opts.OnSequenceGap = func() { gap <- struct{}{} }
	})
	nextBid(t, sub)

	escaped := make(chan any, 1)
	go func() {
		defer func() { escaped <- recover() }()
		_ = sub.SetBucket("0.01")
	}()
	receive(t, blocked.entered, "the SetBucket OnEvent")
	push(liveDelta())
	// The stream worker handles the gap only once its emit for the live
	// delta has returned, so the live view is queued behind the blocked one.
	push(gapDelta())
	receive(t, gap, "the sequence gap after the live view was queued")
	blocked.release()

	if recovered := <-escaped; recovered != nil {
		t.Fatalf("the live view's panic escaped through SetBucket: %v", recovered)
	}
	if err := reportedError(t, reported); !strings.Contains(err.Error(), "apply_live_publications callback panicked: live view panicked") {
		t.Fatalf("OnError got %v", err)
	}
	if sub.Err() == nil || !sub.Closed() {
		t.Fatalf("after the live view panicked: Err() = %v, Closed() = %v", sub.Err(), sub.Closed())
	}
}

// A SetBucket view, queued behind a live view's blocked OnEvent and delivered
// by the stream worker after SetBucket returned, panics in OnEvent. It fails
// the subscription closed with an OnError naming OnEvent; it does not panic
// the worker on behalf of a SetBucket call that has returned.
func TestOrderbookASetBucketViewPanicDeliveredByTheWorkerFailsTheStream(t *testing.T) {
	blocked := newGate(t)
	reported := make(chan error, 1)
	sub, _, push := nestedBucketStream(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		switch {
		case view.BookSeq == "2" && view.Bids[0].Price.Ticks() == nestedRaw:
			blocked.wait()
		case view.BookSeq == "2" && view.Bids[0].Price.Ticks() == nestedMilli:
			panic("bucket view panicked")
		}
	}, func(opts *CreateSubscriptionOptions) {
		opts.OnError = func(err error) { reported <- err }
	})
	nextBid(t, sub)

	push(liveDelta())
	receive(t, blocked.entered, "the live view's OnEvent")
	// Queued behind the live view; SetBucket returns without delivering it.
	if err := sub.SetBucket("0.001"); err != nil {
		t.Fatal(err)
	}
	blocked.release()

	if err := reportedError(t, reported); !strings.Contains(err.Error(), "OnEvent callback panicked: bucket view panicked") {
		t.Fatalf("OnError got %v", err)
	}
	if sub.Err() == nil || !sub.Closed() {
		t.Fatalf("after the bucket view panicked: Err() = %v, Closed() = %v", sub.Err(), sub.Closed())
	}
}

// OnEvent calls SetBucket, and OnEvent for that nested view panics. The
// nested SetBucket has returned by then, so the panic fails the subscription
// closed with an OnError naming OnEvent, and the outer SetBucket, whose own
// view did not panic, returns normally.
func TestOrderbookANestedSetBucketViewPanicFailsTheStream(t *testing.T) {
	var grouped atomic.Bool
	reported := make(chan error, 1)
	sub, _, _ := nestedBucketStream(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		switch {
		case view.Bids[0].Price.Ticks() == nestedCent:
			grouped.Store(true)
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
		case grouped.Load() && view.Bids[0].Price.Ticks() == nestedRaw:
			panic("nested view panicked")
		}
	}, func(opts *CreateSubscriptionOptions) {
		opts.OnError = func(err error) { reported <- err }
	})
	nextBid(t, sub)

	if err := sub.SetBucket("0.01"); err != nil {
		t.Fatal(err)
	}
	if err := reportedError(t, reported); !strings.Contains(err.Error(), "OnEvent callback panicked: nested view panicked") {
		t.Fatalf("OnError got %v", err)
	}
	if sub.Err() == nil || !sub.Closed() {
		t.Fatalf("after the nested view panicked: Err() = %v, Closed() = %v", sub.Err(), sub.Closed())
	}
}

func nextBid(t *testing.T, sub *orderbook.Subscription) int64 {
	t.Helper()
	view, ok := bookTicketNext(t, sub.Updates())
	if !ok {
		t.Fatal("Updates() closed")
	}
	return view.Bids[0].Price.Ticks()
}

// nestedBucketDeliveries reads the snapshot's view, applies SetBucket("0.01")
// and reads changes more views. It returns the bids OnEvent saw and the bids
// Updates() delivered.
func nestedBucketDeliveries(t *testing.T, onView func(*orderbook.Subscription, models.OrderbookData), changes int) (called, updated []int64) {
	t.Helper()
	sub, seen := nestedBucketSubscription(t, onView)
	updated = append(updated, nextBid(t, sub))
	if err := sub.SetBucket("0.01"); err != nil {
		t.Fatal(err)
	}
	for range changes {
		updated = append(updated, nextBid(t, sub))
	}
	return seen(), updated
}

// A SetBucket("") from OnEvent, on the grouped view, adds its view after the
// grouped one on both surfaces: OnEvent and Updates() see one order, and the
// last view is ungrouped, as last requested.
func TestOrderbookSetBucketFromOnEventComesAfterTheViewBeingDelivered(t *testing.T) {
	ungroup := func(sub *orderbook.Subscription, view models.OrderbookData) {
		if view.Bids[0].Price.Ticks() == nestedCent {
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
		}
	}
	called, updated := nestedBucketDeliveries(t, ungroup, 2)
	want := []int64{nestedRaw, nestedCent, nestedRaw}
	if !slices.Equal(called, want) || !slices.Equal(updated, want) {
		t.Fatalf("OnEvent saw %v, Updates() delivered %v, want %v on both", called, updated, want)
	}
}

// Two SetBucket calls from OnEvent add two views, in order, on both surfaces.
func TestOrderbookNestedSetBucketsKeepTheirOrder(t *testing.T) {
	regroup := func(sub *orderbook.Subscription, view models.OrderbookData) {
		if view.Bids[0].Price.Ticks() == nestedCent {
			if err := sub.SetBucket("0.001"); err != nil {
				t.Error(err)
			}
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
		}
	}
	called, updated := nestedBucketDeliveries(t, regroup, 3)
	want := []int64{nestedRaw, nestedCent, nestedMilli, nestedRaw}
	if !slices.Equal(called, want) || !slices.Equal(updated, want) {
		t.Fatalf("OnEvent saw %v, Updates() delivered %v, want %v on both", called, updated, want)
	}
}

// A SetBucket made after OnEvent returned stays in order.
func TestOrderbookSetBucketAfterOnEventStaysInOrder(t *testing.T) {
	called, updated := nestedBucketDeliveries(t, func(*orderbook.Subscription, models.OrderbookData) {}, 1)
	want := []int64{nestedRaw, nestedCent}
	if !slices.Equal(called, want) || !slices.Equal(updated, want) {
		t.Fatalf("OnEvent saw %v, Updates() delivered %v, want %v on both", called, updated, want)
	}
}

// When OnEvent panics, the view it panicked on is not queued, as before, but
// the view queued from it is still delivered on both surfaces, and the panic
// still reaches the SetBucket caller.
func TestOrderbookAViewQueuedBeforeOnEventPanicsIsStillDelivered(t *testing.T) {
	sub, seen := nestedBucketSubscription(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		if view.Bids[0].Price.Ticks() == nestedCent {
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
			panic("callback failed")
		}
	})
	if got := nextBid(t, sub); got != nestedRaw {
		t.Fatalf("snapshot view bid %d", got)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "callback failed" {
				t.Fatalf("recovered %v, want the OnEvent panic", recovered)
			}
		}()
		_ = sub.SetBucket("0.01")
	}()
	if got := nextBid(t, sub); got != nestedRaw {
		t.Fatalf("Updates() delivered %d after the panic, want the queued view %d", got, nestedRaw)
	}
	if err := sub.SetBucket("0.001"); err != nil {
		t.Fatal(err)
	}
	if got := nextBid(t, sub); got != nestedMilli {
		t.Fatalf("Updates() delivered %d, want %d", got, nestedMilli)
	}
	if got, want := seen(), []int64{nestedRaw, nestedCent, nestedRaw, nestedMilli}; !slices.Equal(got, want) {
		t.Fatalf("OnEvent saw %v, want %v", got, want)
	}
}

// An OnEvent that queues a SetBucket and then closes the subscription gets no
// further OnEvent, and nothing is queued on Updates() after Close returns.
func TestOrderbookNoDeliveryAfterOnEventCloses(t *testing.T) {
	sub, seen := nestedBucketSubscription(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		if view.Bids[0].Price.Ticks() == nestedCent {
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
			sub.Close()
		}
	})
	if got := nextBid(t, sub); got != nestedRaw {
		t.Fatalf("snapshot view bid %d", got)
	}
	if err := sub.SetBucket("0.01"); err != nil {
		t.Fatal(err)
	}
	if got, want := seen(), []int64{nestedRaw, nestedCent}; !slices.Equal(got, want) {
		t.Fatalf("OnEvent saw %v after Close, want %v", got, want)
	}
	for view := range sub.Updates() {
		t.Fatalf("Updates() delivered %+v after Close", view)
	}
}

// A view whose enqueue overflows Updates() closes the subscription: the view
// queued from its OnEvent is not delivered.
func TestOrderbookNoDeliveryAfterAnOverflowCloses(t *testing.T) {
	var armed atomic.Bool
	sub, seen := nestedBucketSubscription(t, func(sub *orderbook.Subscription, view models.OrderbookData) {
		if armed.CompareAndSwap(true, false) {
			if err := sub.SetBucket(""); err != nil {
				t.Error(err)
			}
		}
	})
	// The snapshot's view plus 199 more fill the 200-view Updates() buffer.
	for i := range 199 {
		bucket := "0.001"
		if i%2 == 1 {
			bucket = ""
		}
		if err := sub.SetBucket(bucket); err != nil {
			t.Fatal(err)
		}
	}
	armed.Store(true)
	if err := sub.SetBucket("0.01"); err != nil {
		t.Fatal(err)
	}
	var overflow *sdkerrors.QueueOverflowError
	if !stderrors.As(sub.Err(), &overflow) {
		t.Fatalf("Err() = %v, want a queue overflow", sub.Err())
	}
	called := seen()
	if len(called) != 201 || called[200] != nestedCent {
		t.Fatalf("OnEvent saw %d views ending %v, want 201 ending with the overflowing one", len(called), called[len(called)-1:])
	}
}
