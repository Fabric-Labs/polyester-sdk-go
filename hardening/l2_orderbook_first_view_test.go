package hardening_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// scriptedBookHandler answers GetOrderBook from respond(call), call starting at 1.
type scriptedBookHandler struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler
	calls   *atomic.Int64
	respond func(call int64) *orderbookv1.GetOrderBookResponse
}

func (h scriptedBookHandler) GetOrderBook(context.Context, *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	return connect.NewResponse(h.respond(h.calls.Add(1))), nil
}

func availableBook(seq uint64) *orderbookv1.GetOrderBookResponse {
	return &orderbookv1.GetOrderBookResponse{
		SymbolId: 1, BookSeq: seq, Available: true,
		Bids: []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 5}},
		Asks: []*orderbookv1.PriceLevel{{PriceTicks: 101, QtyScaled: 5}},
	}
}

func unavailableBook(seq uint64) *orderbookv1.GetOrderBookResponse {
	return &orderbookv1.GetOrderBookResponse{SymbolId: 1, BookSeq: seq}
}

func scriptedBookService(t *testing.T, calls *atomic.Int64, respond func(int64) *orderbookv1.GetOrderBookResponse) *services.OrderbookService {
	t.Helper()
	ws := hardening.SpawnCentrifugoPublish(nil, make(chan []byte))
	t.Cleanup(ws.Close)
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(scriptedBookHandler{calls: calls, respond: respond})
	mux.Handle(path, handler)
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
	t.Cleanup(rt.Close)
	return services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)
}

func nextBook(t *testing.T, sub interface {
	Updates() <-chan models.OrderbookData
	Err() error
}, within time.Duration) models.OrderbookData {
	t.Helper()
	select {
	case book, ok := <-sub.Updates():
		if !ok {
			t.Fatalf("updates closed early: %v", sub.Err())
		}
		return book
	case <-time.After(within):
		t.Fatalf("no update within %s: %v", within, sub.Err())
	}
	return models.OrderbookData{}
}

// Nothing but the fetched snapshot may be the first view, on the channel and
// the callback alike: a default view would read as a real empty book.
func TestL2OrderbookFirstViewIsTheFetchedSnapshot(t *testing.T) {
	var calls atomic.Int64
	service := scriptedBookService(t, &calls, func(int64) *orderbookv1.GetOrderBookResponse { return availableBook(10) })
	events := make(chan models.OrderbookData, 8)
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{
		Symbol: "BTC-USDT", Depth: 10,
		OnEvent: func(book models.OrderbookData) { events <- book },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	first := nextBook(t, sub, 5*time.Second)
	if first.BookSeq != "10" || !first.Available || len(first.Bids) != 1 {
		t.Fatalf("first view %+v is not the fetched snapshot", first)
	}
	select {
	case book := <-events:
		if book.BookSeq != "10" || !book.Available {
			t.Fatalf("first OnEvent view %+v is not the fetched snapshot", book)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnEvent did not see the snapshot")
	}
	// SetBucket after start still re-renders the current book.
	sub.SetBucket("0.01")
	if again := nextBook(t, sub, 5*time.Second); again.BookSeq != "10" {
		t.Fatalf("SetBucket view %+v", again)
	}
}

// An unavailable snapshot is requested again until the book is available.
func TestL2OrderbookRerequestsUnavailableSnapshot(t *testing.T) {
	var calls atomic.Int64
	service := scriptedBookService(t, &calls, func(call int64) *orderbookv1.GetOrderBookResponse {
		if call < 3 {
			return unavailableBook(0)
		}
		return availableBook(10)
	})
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	first := nextBook(t, sub, 5*time.Second)
	if first.Available || len(first.Bids) != 0 {
		t.Fatalf("first view %+v must report the book unavailable", first)
	}
	// A repeated unavailable answer adds no view: the next one is the book.
	second := nextBook(t, sub, 15*time.Second)
	if !second.Available || second.BookSeq != "10" || len(second.Bids) != 1 {
		t.Fatalf("second view %+v must be the available book", second)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("snapshot requests = %d, want 3", got)
	}
	// Once available, requests stop until something asks for one.
	time.Sleep(1500 * time.Millisecond)
	if got := calls.Load(); got != 3 {
		t.Fatalf("snapshot requests after available = %d, want 3", got)
	}
}

func TestL2OrderbookAvailableSnapshotIsNotRerequested(t *testing.T) {
	var calls atomic.Int64
	service := scriptedBookService(t, &calls, func(int64) *orderbookv1.GetOrderBookResponse { return availableBook(10) })
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	nextBook(t, sub, 5*time.Second)
	time.Sleep(1500 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("snapshot requests = %d, want 1", got)
	}
}

// Closing stops pending requests for an unavailable book.
func TestL2OrderbookCloseStopsRerequests(t *testing.T) {
	var calls atomic.Int64
	service := scriptedBookService(t, &calls, func(int64) *orderbookv1.GetOrderBookResponse { return unavailableBook(0) })
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
	if err != nil {
		t.Fatal(err)
	}
	if view := nextBook(t, sub, 5*time.Second); view.Available {
		t.Fatalf("view %+v must be unavailable", view)
	}
	sub.Close()
	time.Sleep(1500 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("snapshot requests after close = %d, want 1", got)
	}
}

// A managed order book the server ends releases its consumers with the typed
// error, after delivering what is queued.
func TestL2OrderbookServerUnsubscribeReleasesConsumers(t *testing.T) {
	const channel = "public:spot:orderbook:deltas:depth:20:1:proto"
	frames := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoFrames(nil, frames)
	t.Cleanup(ws.Close)
	mux := http.NewServeMux()
	var calls atomic.Int64
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(scriptedBookHandler{calls: &calls, respond: func(int64) *orderbookv1.GetOrderBookResponse { return availableBook(10) }})
	mux.Handle(path, handler)
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
	t.Cleanup(rt.Close)
	service := services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)
	errs := make(chan error, 4)
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{
		Symbol: "BTC-USDT", Depth: 10, OnError: func(err error) { errs <- err },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if view := nextBook(t, sub, 5*time.Second); view.BookSeq != "10" {
		t.Fatalf("view %+v", view)
	}
	frames <- hardening.CentrifugoUnsubscribe(channel, 2000, "server unsubscribe")
	select {
	case _, ok := <-sub.Updates():
		if ok {
			t.Fatal("no further view was published")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumers not released after the server ended the book")
	}
	var ended *sdkerrors.SubscriptionEndedError
	if !errors.As(sub.Err(), &ended) || ended.Channel != channel {
		t.Fatalf("Err() = %v", sub.Err())
	}
	select {
	case reported := <-errs:
		if !errors.As(reported, &ended) {
			t.Fatalf("callback got %v", reported)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("error callback not called")
	}
}

// A newer available snapshot makes a pending request for the old unavailable
// one obsolete: it must not fire, or a failure there could end a healthy stream.
func TestL2OrderbookAvailableSnapshotCancelsPendingRerequest(t *testing.T) {
	var calls atomic.Int64
	service := scriptedBookService(t, &calls, func(call int64) *orderbookv1.GetOrderBookResponse {
		if call == 1 {
			return unavailableBook(0)
		}
		return availableBook(10)
	})
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if view := nextBook(t, sub, 5*time.Second); view.Available {
		t.Fatalf("first view %+v must be unavailable", view)
	}
	if err := sub.RefreshSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if view := nextBook(t, sub, 5*time.Second); !view.Available {
		t.Fatalf("refreshed view %+v must be available", view)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("snapshot requests = %d, want 2 before the old timer is due", got)
	}
	// Past the 1 s delay the unavailable snapshot scheduled.
	time.Sleep(1500 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Fatalf("snapshot requests = %d: an obsolete request fired", got)
	}
	if sub.Err() != nil {
		t.Fatalf("healthy stream has error %v", sub.Err())
	}
}
