package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func bookTicketService(t *testing.T, seq uint64, available bool, wantID ...uint32) (*OrderbookService, chan []byte, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	id := uint32(1)
	if len(wantID) > 0 {
		id = wantID[0]
	}
	sends := make(chan []byte, 10)
	stop := make(chan struct{})
	fail := new(atomic.Bool)
	calls := new(atomic.Int32)
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	ws := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for index, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			var command []byte
			if _, command, err = conn.ReadMessage(); err != nil {
				return
			}
			if index == 1 && !strings.Contains(string(command), fmt.Sprintf(":%d:proto", id)) {
				t.Error("subscription channel identity mismatch")
			}
			if err = conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		for {
			select {
			case data := <-sends:
				if err = conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}))
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		request := new(ob.GetOrderBookRequest)
		if err := proto.Unmarshal(body, request); err != nil {
			t.Error(err)
			return
		}
		if request.SymbolId != id {
			t.Error("snapshot identity mismatch")
		}
		if fail.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"unavailable","message":"snapshot unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{SymbolId: id, BookSeq: seq, Available: available})
		_, _ = w.Write(data)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": 1, "base_quantity_scale": 8},
		map[string]any{"symbol": "ETH-USDT", "symbol_id": 2, "base_quantity_scale": 6},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	return NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt), sends, fail, calls
}
func bookTicketFrame(msg *ob.OrderBookDelta) []byte {
	data, _ := proto.Marshal(msg)
	// These small fixtures fit in single-byte protobuf lengths.
	return append([]byte{byte(len(data) + 6), 34, byte(len(data) + 4), 34, byte(len(data) + 2), 34, byte(len(data))}, data...)
}
func bookTicketNext(t *testing.T, updates <-chan models.OrderbookData) (models.OrderbookData, bool) {
	t.Helper()
	select {
	case data, ok := <-updates:
		return data, ok
	case <-time.After(10 * time.Second):
		t.Fatal("managed receiver did not complete")
		return models.OrderbookData{}, false
	}
}
func TestTicket8510SequenceAvailabilityControls(t *testing.T) {
	for _, seq := range []uint64{0, 1} {
		for _, available := range []bool{false, true} {
			svc, _, _, _ := bookTicketService(t, seq, available)
			unary, err := svc.Get(context.Background(), "BTC-USDT", 50)
			if err != nil {
				t.Fatal(err)
			}
			sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT"})
			if err != nil {
				t.Fatal(err)
			}
			view, ok := bookTicketNext(t, sub.Updates())
			if !ok || view.BookSeq != unary.BookSeq || view.Available != available {
				t.Fatalf("view=%+v unary=%+v", view, unary)
			}
			if seq == 0 && view.BookSeq != "0" {
				t.Fatal("zero sequence lost")
			}
			sub.Close()
		}
	}
}
func TestTicket8512ConflictingSelectorsFailBeforeIO(t *testing.T) {
	svc, _, _, calls := bookTicketService(t, 1, true)
	id := uint32(2)
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT", SymbolID: &id})
	if sub != nil {
		sub.Close()
	}
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "symbol_id") {
		t.Fatalf("expected selector validation, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("conflicting selectors performed snapshot I/O")
	}
}
func TestTicket8511TerminalLifecycle(t *testing.T) {
	for _, mode := range []string{"refresh", "transport", "close", "recover"} {
		t.Run(mode, func(t *testing.T) {
			svc, sends, fail, calls := bookTicketService(t, 1, true)
			reported := make(chan error, 10)
			sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT", OnError: func(err error) { reported <- err }})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			view, ok := bookTicketNext(t, sub.Updates())
			if !ok || view.BookSeq != "1" {
				t.Fatal("healthy initialization did not occur")
			}
			switch mode {
			case "refresh":
				fail.Store(true)
				sends <- bookTicketFrame(&ob.OrderBookDelta{SymbolId: 1, BookSeqStart: 3, BookSeqEnd: 3})
			case "transport":
				sends <- []byte{255}
			case "close":
				sub.Close()
			case "recover":
				fail.Store(true)
				if sub.RefreshSnapshot(context.Background()) == nil {
					t.Fatal("refresh never failed")
				}
				fail.Store(false)
				if err := sub.RefreshSnapshot(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, ok := bookTicketNext(t, sub.Updates()); !ok {
					t.Fatal("recoverable error closed receiver")
				}
				if sub.Err() != nil {
					t.Fatal(sub.Err())
				}
				return
			}
			if _, ok := bookTicketNext(t, sub.Updates()); ok {
				t.Fatal("terminal receiver remained open")
			}
			if mode != "close" {
				if sub.Err() == nil {
					t.Fatal("missing terminal error")
				}
				select {
				case <-reported:
				default:
					t.Fatal("missing callback")
				}
				if mode == "refresh" && calls.Load() < 3 {
					t.Fatal("refresh exhaustion never occurred")
				}
			} else if sub.Err() != nil {
				t.Fatal(sub.Err())
			}
		})
	}
}

func TestTicket8512SelectorControlsKeepResetIdentityAndScale(t *testing.T) {
	for _, control := range []struct {
		symbol   string
		id       uint32
		explicit bool
		quantity string
	}{
		{"BTC-USDT", 1, false, "0.01"}, {"ETH-USDT", 2, false, "1"}, {"ETH-USDT", 2, true, "1"},
	} {
		t.Run(control.symbol+fmt.Sprint(control.explicit), func(t *testing.T) {
			svc, sends, _, _ := bookTicketService(t, 1, true, control.id)
			opts := CreateSubscriptionOptions{Symbol: control.symbol}
			if control.explicit {
				opts.SymbolID = &control.id
			}
			sub, err := svc.CreateSubscription(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			seed, ok := bookTicketNext(t, sub.Updates())
			if !ok || seed.Symbol != control.symbol {
				t.Fatal("missing matching snapshot")
			}
			sends <- bookTicketFrame(&ob.OrderBookDelta{SymbolId: control.id, BookSeqStart: 0, BookSeqEnd: 2, Reset_: true, Bids: []*ob.PriceLevel{{PriceTicks: 1000000000, QtyScaled: 1000000}}})
			reset, ok := bookTicketNext(t, sub.Updates())
			if !ok || reset.Symbol != control.symbol || reset.BookSeq != "2" || len(reset.Bids) != 1 {
				t.Fatalf("reset=%+v", reset)
			}
			got, err := reset.Bids[0].Qty.Format()
			if err != nil || got != control.quantity {
				t.Fatalf("quantity=%s err=%v want=%s", got, err, control.quantity)
			}
		})
	}
}

func TestTicket8510ZeroSequenceOrdering(t *testing.T) {
	for _, mode := range []string{"gap", "contiguous", "reset"} {
		t.Run(mode, func(t *testing.T) {
			svc, sends, _, calls := bookTicketService(t, 0, true)
			gaps := make(chan struct{}, 1)
			sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT", OnSequenceGap: func() { gaps <- struct{}{} }})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			initial, _ := bookTicketNext(t, sub.Updates())
			if initial.BookSeq != "0" {
				t.Fatal(initial)
			}
			seq := uint64(5)
			if mode == "contiguous" {
				seq = 1
			}
			sends <- bookTicketFrame(&ob.OrderBookDelta{BookSeqStart: seq, BookSeqEnd: seq, Reset_: mode == "reset", Bids: []*ob.PriceLevel{{PriceTicks: 100, QtyScaled: 1}}})
			view, ok := bookTicketNext(t, sub.Updates())
			if !ok {
				t.Fatal(sub.Err())
			}
			if mode == "gap" {
				if view.BookSeq != "0" || len(view.Bids) != 0 || calls.Load() != 2 {
					t.Fatalf("gap applied: view=%+v calls=%d", view, calls.Load())
				}
				select {
				case <-gaps:
				default:
					t.Fatal("gap callback missing")
				}
			} else if view.BookSeq != fmt.Sprint(seq) || len(view.Bids) != 1 || calls.Load() != 1 {
				t.Fatalf("control failed: %+v calls=%d", view, calls.Load())
			}
		})
	}
}

func TestTicket8512UnknownMatchingSelectorsRequireCatalogs(t *testing.T) {
	svc, _, _, calls := bookTicketService(t, 1, true)
	svc.catalogs = catalogs.NewManager()
	id := uint32(1)
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT", SymbolID: &id})
	if sub != nil {
		sub.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "Client.WaitForCatalogs") {
		t.Fatalf("expected hydration error, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("opened snapshot before validation")
	}
}

func TestTicket8511OverflowThenClose(t *testing.T) {
	svc, sends, _, _ := bookTicketService(t, 1, true)
	failed := make(chan error, 1)
	events := make(chan models.OrderbookData, 2)
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{
		Symbol:  "BTC-USDT",
		OnError: func(err error) { failed <- err },
		OnEvent: func(data models.OrderbookData) { events <- data },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the managed queue full while pacing transport delivery at the emit
	// boundary. Otherwise a fast producer can overflow the raw transport first.
	if view, ok := bookTicketNext(t, events); !ok || view.BookSeq != "1" {
		t.Fatalf("initial event: %+v", view)
	}
	for seq := uint64(2); seq <= 201; seq++ {
		sends <- bookTicketFrame(&ob.OrderBookDelta{BookSeqStart: seq, BookSeqEnd: seq})
		view, ok := bookTicketNext(t, events)
		if !ok || view.BookSeq != fmt.Sprint(seq) {
			t.Fatalf("live event: %+v, expected %d", view, seq)
		}
	}
	select {
	case err = <-failed:
	case <-time.After(10 * time.Second):
		t.Fatal("managed overflow never occurred")
	}
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(err, &overflow) || !errors.As(sub.Err(), &overflow) {
		t.Fatalf("expected typed overflow: %v / %v", err, sub.Err())
	}
	if err.Error() != "orderbook subscription queue full; consumer too slow" {
		t.Fatalf("wrong overflow boundary: %v", err)
	}

	closed := make(chan struct{})
	go func() { sub.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close deadlocked after overflow")
	}
	count := 0
	for range sub.Updates() {
		count++
	}
	if count != 200 {
		t.Fatalf("queued views lost: %d", count)
	}
}

func TestTicket8512HydrationMessagesNameGoAPI(t *testing.T) {
	svc, _, _, _ := bookTicketService(t, 1, true)
	svc.catalogs = catalogs.NewManager()
	// A legacy partial catalog can know the ID while still lacking its scale.
	svc.catalogs.SpotConfig = map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": 1},
	}}
	_, getErr := svc.Get(context.Background(), "BTC-USDT", 50)
	_, subErr := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT"})
	for _, err := range []error{getErr, subErr} {
		var typed *sdkerrors.ValidationError
		if !errors.As(err, &typed) || !strings.Contains(err.Error(), "Client.WaitForCatalogs") {
			t.Fatalf("expected Go hydration hint, got %v", err)
		}
	}
}
