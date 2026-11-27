package services

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
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
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	done := make(chan struct{})
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
		<-done
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
	created, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "DOGE-USDT", Depth: 20, OnEvent: onEvent})
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

// After OnEvent panics, the views queued behind the one it panicked on are
// dropped with it, and the next change is delivered on both surfaces.
func TestOrderbookViewsAreDeliveredAgainAfterOnEventPanics(t *testing.T) {
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
	if err := sub.SetBucket("0.001"); err != nil {
		t.Fatal(err)
	}
	if got := nextBid(t, sub); got != nestedMilli {
		t.Fatalf("Updates() delivered %d after the panic, want %d", got, nestedMilli)
	}
	if got, want := seen(), []int64{nestedRaw, nestedCent, nestedMilli}; !slices.Equal(got, want) {
		t.Fatalf("OnEvent saw %v, want %v", got, want)
	}
}
