package hardening_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

const (
	rawZeroBid = 71_600_000
	rawZeroAsk = 71_700_000
)

type rawZeroSnapshotHandler struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler
	calls *atomic.Int64
}

func (h rawZeroSnapshotHandler) GetOrderBook(_ context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	if h.calls != nil {
		h.calls.Add(1)
	}
	return connect.NewResponse(&orderbookv1.GetOrderBookResponse{
		SymbolId:  req.Msg.GetSymbolId(),
		BookSeq:   1,
		Available: true,
		Bids:      []*orderbookv1.PriceLevel{{PriceTicks: rawZeroBid, QtyScaled: 3}},
		Asks:      []*orderbookv1.PriceLevel{{PriceTicks: rawZeroAsk, QtyScaled: 4}},
	}), nil
}

type rawZeroStream struct {
	sub       *orderbook.Subscription
	pubs      chan []byte
	errors    *atomic.Int64
	gaps      *atomic.Int64
	snapshots *atomic.Int64
}

func startRawZeroStream(t *testing.T, bucket string, depth int) rawZeroStream {
	t.Helper()
	pubs := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(ws.Close)
	snapshots := &atomic.Int64{}
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(rawZeroSnapshotHandler{calls: snapshots})
	mux.Handle(path, handler)
	api := hardening.NewTestServer(mux)
	t.Cleanup(api.Close)

	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "DOGE-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
	t.Cleanup(rt.Close)
	service := services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)

	errorCount := &atomic.Int64{}
	gaps := &atomic.Int64{}
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{
		Symbol:        "DOGE-USDT",
		Depth:         depth,
		Bucket:        bucket,
		OnError:       func(error) { errorCount.Add(1) },
		OnSequenceGap: func() { gaps.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)

	// The delta is published only after the snapshot view proves the stream is live.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case book, open := <-sub.Updates():
			if !open {
				t.Fatalf("stream closed before its snapshot: %v", sub.Err())
			}
			if book.BookSeq == "1" {
				if len(book.Bids) != 1 || len(book.Asks) != 1 {
					t.Fatalf("snapshot view %+v", book)
				}
				return rawZeroStream{sub: sub, pubs: pubs, errors: errorCount, gaps: gaps, snapshots: snapshots}
			}
		case <-deadline:
			t.Fatalf("no snapshot view: %v", sub.Err())
		}
	}
}

func publishRawZeroDelta(t *testing.T, stream rawZeroStream, side string, quantity int64) {
	t.Helper()
	// An omitted proto3 price decodes to zero.
	level := []*orderbookv1.PriceLevel{{QtyScaled: quantity}}
	delta := &orderbookv1.OrderBookDelta{SymbolId: 1, BookSeqStart: 2, BookSeqEnd: 2}
	if side == "bids" {
		delta.Bids = level
	} else {
		delta.Asks = level
	}
	payload, err := proto.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	stream.pubs <- payload
}

// assertRawZeroResyncs proves a raw zero price was refused like any malformed
// delta: never applied or shown, the book is rebuilt from a fresh snapshot, and
// the stream stays open and keeps applying valid deltas.
func assertRawZeroResyncs(t *testing.T, stream rawZeroStream) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for stream.snapshots.Load() < 2 {
		select {
		case book, open := <-stream.sub.Updates():
			if !open {
				t.Fatalf("raw zero price closed the stream: %v", stream.sub.Err())
			}
			if book.BookSeq != "1" {
				t.Fatalf("raw zero delta was applied: %+v", book)
			}
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("raw zero price did not refresh the snapshot (Err=%v, OnError=%d)", stream.sub.Err(), stream.errors.Load())
		}
	}
	payload, err := proto.Marshal(&orderbookv1.OrderBookDelta{
		SymbolId: 1, BookSeqStart: 2, BookSeqEnd: 2,
		Bids: []*orderbookv1.PriceLevel{{PriceTicks: rawZeroBid, QtyScaled: 7}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream.pubs <- payload
	for {
		select {
		case book, open := <-stream.sub.Updates():
			if !open {
				t.Fatalf("stream closed after resync: %v", stream.sub.Err())
			}
			if book.BookSeq == "1" {
				continue
			}
			if book.BookSeq != "2" || len(book.Bids) != 1 || book.Bids[0].Qty.Scaled() != 7 || len(book.Asks) != 1 {
				t.Fatalf("valid delta after resync: %+v", book)
			}
			if stream.gaps.Load() < 1 || stream.errors.Load() != 0 || stream.sub.Err() != nil {
				t.Fatalf("gaps=%d OnError=%d Err=%v", stream.gaps.Load(), stream.errors.Load(), stream.sub.Err())
			}
			return
		case <-deadline:
			t.Fatalf("valid delta after resync never applied: %v", stream.sub.Err())
		}
	}
}

func TestL2OrderbookRawZeroDeltaResyncsAtEveryBucket(t *testing.T) {
	for _, side := range []string{"bids", "asks"} {
		for _, bucket := range []string{"", "0.001", "0.1"} {
			t.Run(side+"/bucket="+bucket, func(t *testing.T) {
				stream := startRawZeroStream(t, bucket, 10)
				publishRawZeroDelta(t, stream, side, 9)
				assertRawZeroResyncs(t, stream)
			})
		}
	}
}

// A zero bid sorts last, so a depth of one alone would never render it.
func TestL2OrderbookRawZeroBidOutsideRequestedDepthResyncs(t *testing.T) {
	for _, bucket := range []string{"", "0.001", "0.1"} {
		t.Run("bucket="+bucket, func(t *testing.T) {
			stream := startRawZeroStream(t, bucket, 1)
			publishRawZeroDelta(t, stream, "bids", 9)
			assertRawZeroResyncs(t, stream)
		})
	}
}

func TestL2OrderbookRawZeroRemovalIsHarmless(t *testing.T) {
	for _, side := range []string{"bids", "asks"} {
		for _, bucket := range []struct {
			text  string
			ticks int64
		}{{"", 1}, {"0.001", 1_000_000}, {"0.1", 100_000_000}} {
			t.Run(side+"/bucket="+bucket.text, func(t *testing.T) {
				stream := startRawZeroStream(t, bucket.text, 10)
				publishRawZeroDelta(t, stream, side, 0)
				deadline := time.After(5 * time.Second)
				for {
					select {
					case book, open := <-stream.sub.Updates():
						if !open {
							t.Fatalf("removal closed the stream: %v", stream.sub.Err())
						}
						if book.BookSeq != "2" {
							continue
						}
						if len(book.Bids) != 1 || len(book.Asks) != 1 {
							t.Fatalf("removal changed the book: %+v", book)
						}
						wantBid := rawZeroBid / bucket.ticks * bucket.ticks
						wantAsk := (rawZeroAsk + bucket.ticks - 1) / bucket.ticks * bucket.ticks
						if book.Bids[0].Price.Ticks() != wantBid || book.Asks[0].Price.Ticks() != wantAsk {
							t.Fatalf("removal changed prices: %+v", book)
						}
						if book.Bids[0].Qty.Scaled() != 3 || book.Asks[0].Qty.Scaled() != 4 {
							t.Fatalf("removal changed quantities: %+v", book)
						}
						if err := stream.sub.Err(); err != nil || stream.errors.Load() != 0 {
							t.Fatalf("removal reported an error: %v (%d)", err, stream.errors.Load())
						}
						return
					case <-deadline:
						t.Fatalf("removal delta was never applied: %v", stream.sub.Err())
					}
				}
			})
		}
	}
}

// A positive bid floored to the zero boundary is a valid view, on snapshot and delta.
func TestL2OrderbookCoarseBucketKeepsFlooredZeroBid(t *testing.T) {
	stream := startRawZeroStream(t, "0.1", 10)
	delta := &orderbookv1.OrderBookDelta{
		SymbolId: 1, BookSeqStart: 2, BookSeqEnd: 2,
		Bids: []*orderbookv1.PriceLevel{{PriceTicks: rawZeroBid, QtyScaled: 5}},
	}
	payload, err := proto.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	stream.pubs <- payload
	deadline := time.After(5 * time.Second)
	for {
		select {
		case book, open := <-stream.sub.Updates():
			if !open {
				t.Fatalf("floored bid closed the stream: %v", stream.sub.Err())
			}
			if book.BookSeq != "2" {
				continue
			}
			if len(book.Bids) != 1 || book.Bids[0].Price.Ticks() != 0 || book.Bids[0].Qty.Scaled() != 5 {
				t.Fatalf("floored bid view %+v", book)
			}
			if len(book.Asks) != 1 || book.Asks[0].Price.Ticks() != 100_000_000 {
				t.Fatalf("ask ceiling view %+v", book)
			}
			if stream.errors.Load() != 0 {
				t.Fatalf("floored bid reported an error")
			}
			return
		case <-deadline:
			t.Fatalf("delta was never applied: %v", stream.sub.Err())
		}
	}
}

type delayedSnapshotHandler struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler
	started chan struct{}
	release chan struct{}
	once    *sync.Once
	calls   *atomic.Int64
}

func (h delayedSnapshotHandler) GetOrderBook(ctx context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	h.calls.Add(1)
	h.once.Do(func() { close(h.started) })
	select {
	case <-h.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return rawZeroSnapshotHandler{}.GetOrderBook(ctx, req)
}

// A raw zero price buffered ahead of the snapshot is refused like a live one:
// neither it nor the delta behind it is applied, and the book is rebuilt from
// a fresh snapshot instead of closing.
func TestL2OrderbookRawZeroBufferedDeltaResyncsWithoutApplying(t *testing.T) {
	pubs := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(ws.Close)
	snapshot := delayedSnapshotHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
		once:    &sync.Once{},
		calls:   &atomic.Int64{},
	}
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(snapshot)
	mux.Handle(path, handler)
	api := hardening.NewTestServer(mux)
	t.Cleanup(api.Close)

	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "DOGE-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
	t.Cleanup(rt.Close)
	service := services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)

	var mu sync.Mutex
	var seqs []string
	var errorCount atomic.Int64
	type result struct {
		sub *orderbook.Subscription
		err error
	}
	created := make(chan result, 1)
	go func() {
		sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{
			Symbol: "DOGE-USDT",
			Depth:  10,
			OnEvent: func(book models.OrderbookData) {
				mu.Lock()
				seqs = append(seqs, book.BookSeq)
				mu.Unlock()
			},
			OnError: func(error) { errorCount.Add(1) },
		})
		created <- result{sub, err}
	}()

	select {
	case <-snapshot.started:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot was never requested")
	}
	for _, delta := range []*orderbookv1.OrderBookDelta{
		{SymbolId: 1, BookSeqStart: 2, BookSeqEnd: 2, Bids: []*orderbookv1.PriceLevel{{QtyScaled: 9}}},
		{SymbolId: 1, BookSeqStart: 3, BookSeqEnd: 3, Bids: []*orderbookv1.PriceLevel{{QtyScaled: 0}}},
	} {
		payload, err := proto.Marshal(delta)
		if err != nil {
			t.Fatal(err)
		}
		pubs <- payload
	}
	// Both publications must reach the stream's buffer before the snapshot completes.
	time.Sleep(200 * time.Millisecond)
	close(snapshot.release)

	var outcome result
	select {
	case outcome = <-created:
	case <-time.After(5 * time.Second):
		t.Fatal("CreateSubscription did not return")
	}
	if outcome.err != nil {
		t.Fatalf("CreateSubscription: %v", outcome.err)
	}
	t.Cleanup(outcome.sub.Close)
	deadline := time.After(5 * time.Second)
	for snapshot.calls.Load() < 2 {
		select {
		case _, open := <-outcome.sub.Updates():
			if !open {
				t.Fatalf("buffered raw zero closed the stream: %v", outcome.sub.Err())
			}
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("buffered raw zero did not refresh the snapshot (Err=%v)", outcome.sub.Err())
		}
	}
	outcome.sub.Close()
	if err := outcome.sub.Err(); err != nil {
		t.Fatalf("Err=%v", err)
	}
	if got := errorCount.Load(); got != 0 {
		t.Fatalf("OnError called %d times, want 0", got)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, seq := range seqs {
		if seq != "0" && seq != "1" {
			t.Fatalf("a buffered delta was applied: seqs=%v", seqs)
		}
	}
}
