package hardening_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The instants the server stamps on the fixed snapshot and updates, at the
// contract's nanosecond precision.
var (
	snapshotTs = time.Date(2026, 11, 19, 18, 40, 33, 60385485, time.UTC)
	deltaTs    = time.Date(2026, 11, 19, 18, 40, 34, 123456789, time.UTC)
)

type timestampedSnapshotHandler struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler
	ts *timestamppb.Timestamp
}

func (h timestampedSnapshotHandler) GetOrderBook(_ context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	return connect.NewResponse(&orderbookv1.GetOrderBookResponse{
		SymbolId:  req.Msg.GetSymbolId(),
		BookSeq:   7,
		BookEpoch: 2,
		Available: true,
		Depth:     servedTier(req.Msg),
		Bids:      []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 5}},
		Asks:      []*orderbookv1.PriceLevel{{PriceTicks: 101, QtyScaled: 6}},
		Ts:        h.ts,
	}), nil
}

func timestampCases() map[string]*time.Time {
	return map[string]*time.Time{"present": &snapshotTs, "absent": nil}
}

func timestampProto(ts *time.Time) *timestamppb.Timestamp {
	if ts == nil {
		return nil
	}
	return timestamppb.New(*ts)
}

func assertTimestamp(t *testing.T, what string, got, want *time.Time) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("%s timestamp = %v, want nil: the server sent none", what, *got)
	case want != nil && got == nil:
		t.Fatalf("%s timestamp = nil, want %v", what, *want)
	case want != nil && !got.Equal(*want):
		t.Fatalf("%s timestamp = %v, want %v", what, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func newTimestampOrderbookService(t *testing.T, snapshot *time.Time, pubs <-chan []byte) *services.OrderbookService {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(timestampedSnapshotHandler{ts: timestampProto(snapshot)})
	mux.Handle(path, handler)
	api := hardening.NewTestServer(mux)
	t.Cleanup(api.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	var rt services.RealtimeClient
	if pubs != nil {
		ws := hardening.SpawnCentrifugoPublish(nil, pubs)
		t.Cleanup(ws.Close)
		client := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
		t.Cleanup(client.Close)
		rt = client
	}
	return services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)
}

func deltaPayload(t *testing.T, delta *orderbookv1.OrderBookDelta) []byte {
	t.Helper()
	payload, err := proto.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// Orderbook.Get reports the snapshot's best-effort timestamp when the server
// supplies one, and nil, never the local receipt time, when it does not.
func TestL2OrderbookGetCarriesSnapshotTimestamp(t *testing.T) {
	for name, want := range timestampCases() {
		t.Run(name, func(t *testing.T) {
			service := newTimestampOrderbookService(t, want, nil)
			book, err := service.Get(context.Background(), "BTC-USDT", 1)
			if err != nil {
				t.Fatal(err)
			}
			assertTimestamp(t, "snapshot", book.Ts, want)
			if book.Symbol != "BTC-USDT" || book.Depth != 1 || book.BookSeq != "7" || book.BookEpoch != "2" || !book.Available ||
				len(book.Bids) != 1 || len(book.Asks) != 1 {
				t.Fatalf("book %+v", book)
			}
		})
	}
}

// Raw deltas carry each update's timestamp when the server supplies one, and
// nil when it does not.
func TestL2OrderbookDeltasCarryUpdateTimestamp(t *testing.T) {
	for name, want := range map[string]*time.Time{"present": &deltaTs, "absent": nil} {
		t.Run(name, func(t *testing.T) {
			pubs := make(chan []byte, 1)
			service := newTimestampOrderbookService(t, nil, pubs)
			sub, err := service.SubscribeDeltas(context.Background(), 1, 20)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			pubs <- deltaPayload(t, &orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 7, BookSeqEnd: 8,
				Bids: []*orderbookv1.PriceLevel{{PriceTicks: 99, QtyScaled: 7}},
				Ts:   timestampProto(want),
			})
			select {
			case delta := <-sub.Messages():
				assertTimestamp(t, "delta", delta.Ts, want)
				if delta.BookSeqStart != "7" || delta.BookSeqEnd != "8" || len(delta.Bids) != 1 {
					t.Fatalf("delta %+v", delta)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no delta delivered")
			}
		})
	}
}

func nextView(t *testing.T, updates <-chan models.OrderbookData, seq string, errFn func() error) models.OrderbookData {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case view := <-updates:
			if view.BookSeq == seq {
				return view
			}
		case <-deadline:
			t.Fatalf("no view at book sequence %s: %v", seq, errFn())
		}
	}
}

// A managed book's view carries the timestamp of the latest snapshot or
// update applied to it: the snapshot's until an update applies, then that
// update's. A snapshot or update without one leaves the view's timestamp nil.
func TestL2OrderbookSubscriptionCarriesLatestAppliedTimestamp(t *testing.T) {
	for name, snapshot := range timestampCases() {
		t.Run(name, func(t *testing.T) {
			pubs := make(chan []byte, 4)
			service := newTimestampOrderbookService(t, snapshot, pubs)
			sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()

			view := nextView(t, sub.Updates(), "7", sub.Err)
			assertTimestamp(t, "snapshot view", view.Ts, snapshot)

			pubs <- deltaPayload(t, &orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 7, BookSeqEnd: 8,
				Bids: []*orderbookv1.PriceLevel{{PriceTicks: 99, QtyScaled: 7}},
				Ts:   timestamppb.New(deltaTs),
			})
			view = nextView(t, sub.Updates(), "8", sub.Err)
			assertTimestamp(t, "timestamped update view", view.Ts, &deltaTs)

			// A view's timestamp is its own: changing it leaves the book's.
			*view.Ts = time.Time{}

			// A stale update is not applied, so the view it renders keeps the
			// applied update's timestamp.
			pubs <- deltaPayload(t, &orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 6, BookSeqEnd: 7,
				Ts: timestamppb.New(snapshotTs.Add(-time.Hour)),
			})
			view = nextView(t, sub.Updates(), "8", sub.Err)
			assertTimestamp(t, "stale update view", view.Ts, &deltaTs)

			pubs <- deltaPayload(t, &orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 8, BookSeqEnd: 9,
				Bids: []*orderbookv1.PriceLevel{{PriceTicks: 98, QtyScaled: 3}},
				Ts:   timestamppb.New(deltaTs.Add(time.Second)),
			})
			view = nextView(t, sub.Updates(), "9", sub.Err)
			later := deltaTs.Add(time.Second)
			assertTimestamp(t, "second update view", view.Ts, &later)

			pubs <- deltaPayload(t, &orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 9, BookSeqEnd: 10,
				Asks: []*orderbookv1.PriceLevel{{PriceTicks: 102, QtyScaled: 4}},
			})
			view = nextView(t, sub.Updates(), "10", sub.Err)
			assertTimestamp(t, "untimestamped update view", view.Ts, nil)
		})
	}
}
