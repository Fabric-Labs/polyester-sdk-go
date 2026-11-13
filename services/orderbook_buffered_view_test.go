package services

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// POLY-9098: a snapshot applied with the deltas buffered behind it is one book
// state, so it yields one view. A view per buffered delta plus one more left
// stale copies queued, and the next bucket change read one of them.
func TestSnapshotWithBufferedDeltasEmitsOneView(t *testing.T) {
	for _, buffered := range []uint64{1, 3} {
		t.Run(strconv.FormatUint(buffered, 10)+" buffered", func(t *testing.T) {
			// Deltas 2..1+buffered follow the subscribe ack at once, while the snapshot
			// (sequence 1) is held back, so all of them are buffered.
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
				for seq := uint64(2); seq < 2+buffered; seq++ {
					frame := bookTicketFrame(&ob.OrderBookDelta{
						SymbolId: 1, BookSeqStart: seq - 1, BookSeqEnd: seq,
						Bids: []*ob.PriceLevel{{PriceTicks: 71_600_000, QtyScaled: int64(seq)}},
					})
					if err = conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
						return
					}
				}
				<-done
			}))
			httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(300 * time.Millisecond)
				w.Header().Set("Content-Type", "application/proto")
				data, _ := proto.Marshal(&ob.GetOrderBookResponse{
					SymbolId: 1, BookSeq: 1, Available: true,
					Bids: []*ob.PriceLevel{{PriceTicks: 71_600_000, QtyScaled: 1}},
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

			sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "DOGE-USDT", Depth: 50, Bucket: "0.1"})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			first, ok := bookTicketNext(t, sub.Updates())
			if !ok || first.BookSeq != strconv.FormatUint(1+buffered, 10) || first.Bids[0].Qty.Scaled() != int64(1+buffered) {
				t.Fatalf("first view %+v, want the book at sequence %d", first, 1+buffered)
			}
			select {
			case extra := <-sub.Updates():
				t.Fatalf("%d buffered deltas: one book state emitted a second view %+v", buffered, extra)
			case <-time.After(300 * time.Millisecond):
			}

			// The next bucket change reads its own view, not a stale one.
			sub.SetBucket("0.001")
			fine, ok := bookTicketNext(t, sub.Updates())
			if !ok || fine.Bids[0].Price.Ticks() != 71_000_000 {
				t.Fatalf("bid bucket price after the bucket change: %+v, want 71000000", fine.Bids)
			}
		})
	}
}
