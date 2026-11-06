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
)

type orderbookSnapshotHandler struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler
	available bool
}

func (h orderbookSnapshotHandler) GetOrderBook(_ context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	resp := &orderbookv1.GetOrderBookResponse{SymbolId: req.Msg.GetSymbolId(), BookSeq: 10, Available: h.available}
	if h.available {
		resp.Bids = []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 5}}
		resp.Asks = []*orderbookv1.PriceLevel{{PriceTicks: 101, QtyScaled: 5}}
	}
	return connect.NewResponse(resp), nil
}

// The managed order book carries its snapshot's Available on every update:
// a book merged onto an unavailable snapshot, or rendered before its first
// snapshot, is not reported as current.
func TestL2OrderbookSubscriptionCarriesSnapshotAvailability(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "unavailable"}[available], func(t *testing.T) {
			pubs := make(chan []byte, 4)
			ws := hardening.SpawnCentrifugoPublish(nil, pubs)
			t.Cleanup(ws.Close)
			mux := http.NewServeMux()
			path, handler := orderbookv1connect.NewOrderbookServiceHandler(orderbookSnapshotHandler{available: available})
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
			rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
			t.Cleanup(rt.Close)
			service := services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)

			sub, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 10})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()

			payload, err := proto.Marshal(&orderbookv1.OrderBookDelta{
				SymbolId: 1, BookSeqStart: 11, BookSeqEnd: 11,
				Bids: []*orderbookv1.PriceLevel{{PriceTicks: 99, QtyScaled: 7}},
			})
			if err != nil {
				t.Fatal(err)
			}
			pubs <- payload

			deadline := time.After(5 * time.Second)
			for {
				var book models.OrderbookData
				select {
				case book = <-sub.Updates():
				case <-deadline:
					t.Fatalf("no merged update: %v", sub.Err())
				}
				if book.BookSeq == "0" {
					// SetBucket renders the book before its first snapshot:
					// nothing is known about it yet.
					if book.Available {
						t.Fatalf("book before its snapshot %+v reports available", book)
					}
					continue
				}
				if book.Available != available {
					t.Fatalf("book %+v, want available=%v", book, available)
				}
				if book.BookSeq == "11" {
					return
				}
			}
		})
	}
}
