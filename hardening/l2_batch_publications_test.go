package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
)

// batchStream serves token endpoints and a Centrifugo socket that publishes
// each pushed payload after the subscribe handshake.
type batchStream struct {
	pubs chan []byte
	rt   *realtime.Client
}

func newBatchStream(t *testing.T) *batchStream {
	t.Helper()
	pubs := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(ws.Close)
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"t"}`))
	}))
	t.Cleanup(tokens.Close)
	rt := newRT(ws.WSURL(), tokens.URL, testCreds(t), 5*time.Second)
	t.Cleanup(rt.Close)
	return &batchStream{pubs: pubs, rt: rt}
}

func (b *batchStream) publish(t *testing.T, msg proto.Message) {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b.pubs <- payload
}

func receiveN[T any](t *testing.T, sub *realtime.Subscription[T], n int) []T {
	t.Helper()
	out := make([]T, 0, n)
	for len(out) < n {
		select {
		case item, ok := <-sub.Messages():
			if !ok {
				t.Fatalf("subscription ended after %d of %d events: %v", len(out), n, sub.Err())
			}
			out = append(out, item)
		case <-time.After(5 * time.Second):
			t.Fatalf("received %d of %d events", len(out), n)
		}
	}
	return out
}

func TestL2MarketTradeBatchEmitsEachTradeInOrder(t *testing.T) {
	stream := newBatchStream(t)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	symbolID := uint32(1)
	sub, err := services.NewMarketDataService(nil, cats, stream.rt).SubscribeTrades(context.Background(), nil, &symbolID)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	stream.publish(t, &marketdatav1.MarketTradeBatch{Events: []*marketdatav1.MarketTrade{
		{SymbolId: 1, MatchId: 10, IsBuy: true, PriceTicks: 100_000_000_000, QtyScaled: 150_000_000, TsNs: 1},
		{SymbolId: 1, MatchId: 11, PriceTicks: 101_000_000_000, QtyScaled: 1, TsNs: 2},
	}})
	stream.publish(t, &marketdatav1.MarketTradeBatch{Events: []*marketdatav1.MarketTrade{
		{SymbolId: 1, MatchId: 12, IsBuy: true, PriceTicks: 102_000_000_000, QtyScaled: 2, TsNs: 3},
	}})
	got := receiveN(t, sub, 3)
	for i, want := range []struct {
		matchID, side, qty string
		priceTicks         int64
	}{
		{"10", "buy", "1.5", 100_000_000_000},
		{"11", "sell", "0.00000001", 101_000_000_000},
		{"12", "buy", "0.00000002", 102_000_000_000},
	} {
		qty, err := got[i].Qty.Format()
		if err != nil {
			t.Fatal(err)
		}
		if got[i].SymbolID != 1 || got[i].MatchID != want.matchID || got[i].Side != want.side ||
			got[i].Price.Ticks() != want.priceTicks || qty != want.qty {
			t.Fatalf("trade %d = %+v (qty %s), want %+v", i, got[i], qty, want)
		}
	}
}

func TestL2UserTradeBatchEmitsEachFillInOrder(t *testing.T) {
	stream := newBatchStream(t)
	account := codecs.FormatID(42)
	sub, err := services.NewTradesService(nil, nil, nil, stream.rt, &account).Subscribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	stream.publish(t, &orderv1.UserTradeBatch{Events: []*orderv1.UserTrade{
		{SymbolId: 1, MatchId: 20, OrderId: 7, Side: orderv1.Side_BUY, TsNs: 5},
		{SymbolId: 1, MatchId: 21, OrderId: 8, Side: orderv1.Side_SELL, IsMaker: true, TsNs: 6},
		{SymbolId: 2, MatchId: 22, OrderId: 9, Side: orderv1.Side_BUY, TsNs: 7},
	}})
	got := receiveN(t, sub, 3)
	for i, want := range []struct {
		matchID, orderID, side string
		maker                  bool
	}{
		{"20", codecs.FormatID(7), "buy", false},
		{"21", codecs.FormatID(8), "sell", true},
		{"22", codecs.FormatID(9), "buy", false},
	} {
		if got[i].MatchID != want.matchID || got[i].OrderID != want.orderID || got[i].Side != want.side || got[i].IsMaker != want.maker {
			t.Fatalf("fill %d = %+v, want %+v", i, got[i], want)
		}
	}
}

func TestL2LedgerTransferBatchEmitsEachLegInOrder(t *testing.T) {
	stream := newBatchStream(t)
	account := codecs.FormatID(42)
	sub, err := services.NewTransfersService(nil, nil, stream.rt, &account).Subscribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	stream.publish(t, &ledgerrdv1.TransferRowBatch{Events: []*ledgerrdv1.TransferRow{
		{AssetId: 1, TsUs: 100, IsDebit: true, FlowId: "f1", TransferId: "340282366920938463463374607431768211455"},
		{AssetId: 2, TsUs: 100, FlowId: "f1", TransferId: "340282366920938463463374607431768211455"},
	}})
	got := receiveN(t, sub, 2)
	if got[0].AssetID != 1 || !got[0].IsDebit || got[1].AssetID != 2 || got[1].IsDebit {
		t.Fatalf("legs = %+v", got)
	}
	for i, leg := range got {
		if leg.TransferID != "340282366920938463463374607431768211455" || leg.TxID != "f1" {
			t.Fatalf("leg %d = %+v", i, leg)
		}
	}
}

func TestL2TriggerEventBatchEmitsEachEventInOrder(t *testing.T) {
	stream := newBatchStream(t)
	account := codecs.FormatID(42)
	sub, err := services.NewTriggersService(nil, nil, nil, stream.rt, &account, nil, nil).SubscribeEvents(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	stream.publish(t, &triggersv1.TriggerEventBatch{Events: []*triggersv1.TriggerEvent{
		{TriggerId: 3, SymbolId: 1, EventType: triggersv1.TriggerEventType_EVENT_ACTIVATED, TsNs: 1_700_000_000_000_000_001},
		{TriggerId: 3, SymbolId: 1, EventType: triggersv1.TriggerEventType_EVENT_FIRED, TsNs: 1_700_000_000_000_000_002},
	}})
	got := receiveN(t, sub, 2)
	if got[0].EventType != "activated" || got[0].TsNs != "1700000000000000001" ||
		got[1].EventType != "fired" || got[1].TsNs != "1700000000000000002" {
		t.Fatalf("events = %+v", got)
	}
}
