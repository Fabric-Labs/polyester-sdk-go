package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
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
}

func (rawZeroSnapshotHandler) GetOrderBook(_ context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	return connect.NewResponse(&orderbookv1.GetOrderBookResponse{
		SymbolId:  req.Msg.GetSymbolId(),
		BookSeq:   1,
		Available: true,
		Bids:      []*orderbookv1.PriceLevel{{PriceTicks: rawZeroBid, QtyScaled: 3}},
		Asks:      []*orderbookv1.PriceLevel{{PriceTicks: rawZeroAsk, QtyScaled: 4}},
	}), nil
}

type rawZeroStream struct {
	sub    *orderbook.Subscription
	pubs   chan []byte
	errors *atomic.Int64
}

func startRawZeroStream(t *testing.T, bucket string, depth int) rawZeroStream {
	t.Helper()
	pubs := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(ws.Close)
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(rawZeroSnapshotHandler{})
	mux.Handle(path, handler)
	api := httptest.NewServer(mux)
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
	sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{
		Symbol:  "DOGE-USDT",
		Depth:   depth,
		Bucket:  bucket,
		OnError: func(error) { errorCount.Add(1) },
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
				return rawZeroStream{sub: sub, pubs: pubs, errors: errorCount}
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

func assertRawZeroFailsClosed(t *testing.T, stream rawZeroStream) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case book, open := <-stream.sub.Updates():
			if !open {
				err := stream.sub.Err()
				if err == nil || !strings.Contains(err.Error(), "invalid or missing price") {
					t.Fatalf("terminal error %v", err)
				}
				// Updates closes just before the callback runs on the stream worker;
				// Close waits for that worker, so the count is final afterwards.
				stream.sub.Close()
				if got := stream.errors.Load(); got != 1 {
					t.Fatalf("OnError called %d times, want 1", got)
				}
				return
			}
			if book.BookSeq != "1" {
				t.Fatalf("corrupt delta was delivered: %+v", book)
			}
		case <-deadline:
			t.Fatalf("stream stayed open after a raw zero price (Err=%v, OnError=%d)", stream.sub.Err(), stream.errors.Load())
		}
	}
}

func TestL2OrderbookRawZeroDeltaFailsClosedAtEveryBucket(t *testing.T) {
	for _, side := range []string{"bids", "asks"} {
		for _, bucket := range []string{"", "0.001", "0.1"} {
			t.Run(side+"/bucket="+bucket, func(t *testing.T) {
				stream := startRawZeroStream(t, bucket, 10)
				publishRawZeroDelta(t, stream, side, 9)
				assertRawZeroFailsClosed(t, stream)
			})
		}
	}
}

// A zero bid sorts last, so a depth of one alone would never render it.
func TestL2OrderbookRawZeroBidOutsideRequestedDepthFailsClosed(t *testing.T) {
	for _, bucket := range []string{"", "0.001", "0.1"} {
		t.Run("bucket="+bucket, func(t *testing.T) {
			stream := startRawZeroStream(t, bucket, 1)
			publishRawZeroDelta(t, stream, "bids", 9)
			assertRawZeroFailsClosed(t, stream)
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
