package services

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const (
	snapshotTradeTs = 1795721087978971514
	olderTradeTs    = 1795721087185742363
	newerTradeTs    = 1795721088000000000
)

// overviewBatchFrame wraps a MarketOverviewBatch as the publication frame the
// realtime client reads.
func overviewBatchFrame(batch *overviewpb.MarketOverviewBatch) []byte {
	data, _ := proto.Marshal(batch)
	wrap := func(inner []byte) []byte {
		out := protowire.AppendTag(nil, 4, protowire.BytesType)
		return protowire.AppendBytes(out, inner)
	}
	frame := wrap(wrap(wrap(data)))
	return append(protowire.AppendVarint(nil, uint64(len(frame))), frame...)
}

// firstOverviewView subscribes to a market overview whose snapshot fetch is
// delayed until buffered has been published on the socket, so the
// publication is buffered and merged into snapshot. It returns the first view
// the subscription emits, by symbol ID.
func firstOverviewView(t *testing.T, snapshot []*overviewpb.MarketOverview, buffered []*overviewpb.MarketOverview) map[uint32]models.MarketOverviewEntry {
	t.Helper()
	published := make(chan struct{})
	stop := make(chan struct{})
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
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
		if err := conn.WriteMessage(websocket.BinaryMessage, overviewBatchFrame(&overviewpb.MarketOverviewBatch{Markets: buffered, TsNs: olderTradeTs})); err != nil {
			return
		}
		close(published)
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		<-stop
	}))
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The snapshot answers after the publication went out: the client
		// buffers it while it waits.
		select {
		case <-published:
		case <-time.After(10 * time.Second):
		}
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/proto")
		body, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{Markets: snapshot})
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	factory := transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil)
	svc := NewMarketOverviewService(factory, catalogs.NewManager(), rt)
	views := make(chan []models.MarketOverviewEntry, 4)
	sub, err := svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func(rows []models.MarketOverviewEntry) { views <- rows },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	select {
	case rows := <-views:
		out := map[uint32]models.MarketOverviewEntry{}
		for _, row := range rows {
			out[row.SymbolID] = row
		}
		return out
	case <-time.After(10 * time.Second):
		t.Fatal("no view emitted")
		return nil
	}
}

func overviewRow(symbolID uint32, tradeTs uint64, last int64) *overviewpb.MarketOverview {
	return &overviewpb.MarketOverview{SymbolId: symbolID, LastTradeTsNs: tradeTs, LastPriceTicks: last}
}

// A row buffered while the snapshot loaded whose last trade is older than the
// snapshot's does not replace the snapshot's newer row.
func TestOverviewOlderBufferedRowDoesNotRegressTheSnapshot(t *testing.T) {
	view := firstOverviewView(t,
		[]*overviewpb.MarketOverview{overviewRow(1, snapshotTradeTs, 61_000)},
		[]*overviewpb.MarketOverview{overviewRow(1, olderTradeTs, 60_000), overviewRow(2, olderTradeTs, 3_000)})
	if got := view[1].LastTradeTsNs; got != "1795721087978971514" {
		t.Fatalf("symbol 1 last_trade_ts_ns = %s, want the snapshot's", got)
	}
	if _, ok := view[2]; !ok {
		t.Fatal("a buffered market missing from the snapshot was not added")
	}
}

// A newer buffered row replaces the snapshot's.
func TestOverviewNewerBufferedRowReplacesTheSnapshot(t *testing.T) {
	view := firstOverviewView(t,
		[]*overviewpb.MarketOverview{overviewRow(1, snapshotTradeTs, 61_000)},
		[]*overviewpb.MarketOverview{overviewRow(1, newerTradeTs, 62_000)})
	if got := view[1].LastTradeTsNs; got != "1795721088000000000" {
		t.Fatalf("symbol 1 last_trade_ts_ns = %s, want the buffered row's", got)
	}
}

// A buffered row with the same last-trade time replaces the snapshot's, with
// the index and volume changes made since.
func TestOverviewEqualTradeTimeBufferedRowAppliesIndexAndVolume(t *testing.T) {
	volume := int64(250)
	snapshotVolume := int64(100)
	snap := overviewRow(1, snapshotTradeTs, 61_000)
	snap.IndexPriceTicks, snap.Volume_24HUsdScaled = 60_990, &snapshotVolume
	update := overviewRow(1, snapshotTradeTs, 61_000)
	update.IndexPriceTicks, update.Volume_24HUsdScaled = 61_005, &volume
	view := firstOverviewView(t, []*overviewpb.MarketOverview{snap}, []*overviewpb.MarketOverview{update})
	row := view[1]
	if row.IndexPrice.Ticks() != 61_005 {
		t.Fatalf("index price = %v, want the buffered row's", row.IndexPrice)
	}
	if row.Volume24HUsdScaled == nil || *row.Volume24HUsdScaled != "250" {
		t.Fatalf("24h USD volume = %v, want the buffered row's", row.Volume24HUsdScaled)
	}
}

// A publication that arrives while the snapshot is being applied is merged
// before the stream is ready, as part of that snapshot: an older row does not
// replace the snapshot's newer one. Once the stream is ready, live updates
// apply as they come.
func TestOverviewMergeKeepsTheGuardWhileTheSnapshotSettles(t *testing.T) {
	merged := newOverviewMerge(func(models.MarketOverviewEntry) bool { return true })
	snapshotRow := models.MarketOverviewEntry{SymbolID: 1, LastTradeTsNs: "1795721087978971514"}
	older := models.MarketOverviewEntry{SymbolID: 1, LastTradeTsNs: "1795721087185742363"}
	equal := models.MarketOverviewEntry{SymbolID: 1, LastTradeTsNs: "1795721087978971514", Change24hBps: 7}
	merged.applySnapshot(models.MarketOverviewList{Markets: []models.MarketOverviewEntry{snapshotRow}}, nil)

	merged.applyLive([]models.MarketOverviewList{{Markets: []models.MarketOverviewEntry{older}}}, true)
	if got := merged.rows()[0].LastTradeTsNs; got != snapshotRow.LastTradeTsNs {
		t.Fatalf("settling: an older row replaced the snapshot row (%s)", got)
	}
	merged.applyLive([]models.MarketOverviewList{{Markets: []models.MarketOverviewEntry{equal}}}, true)
	if got := merged.rows()[0].Change24hBps; got != 7 {
		t.Fatalf("settling: an equal-trade-time row did not apply (%d)", got)
	}
	// Ready: live updates apply as before.
	merged.applyLive([]models.MarketOverviewList{{Markets: []models.MarketOverviewEntry{older}}}, false)
	if got := merged.rows()[0].LastTradeTsNs; got != older.LastTradeTsNs {
		t.Fatalf("ready: a live row did not apply (%s)", got)
	}
}

// End to end: the first view's OnEvent runs inside the snapshot's apply; a
// publication sent then reaches the subscription mid-apply and is merged
// before the stream is ready. Its older row does not replace the snapshot's.
func TestOverviewPublicationArrivingMidApplyDoesNotRegressTheSnapshot(t *testing.T) {
	send := make(chan []byte)
	stop := make(chan struct{})
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
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
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case frame := <-send:
				if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}))
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		body, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{Markets: []*overviewpb.MarketOverview{overviewRow(1, snapshotTradeTs, 61_000)}})
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	factory := transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil)
	svc := NewMarketOverviewService(factory, catalogs.NewManager(), rt)
	views := make(chan []models.MarketOverviewEntry, 4)
	first := true
	sub, err := svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func(rows []models.MarketOverviewEntry) {
			if first {
				first = false
				// Still inside the snapshot's apply: the publication arrives now
				// and waits for the apply to finish.
				send <- overviewBatchFrame(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{overviewRow(1, olderTradeTs, 60_000)}})
				time.Sleep(300 * time.Millisecond)
			}
			views <- rows
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	for i := 0; i < 2; i++ {
		select {
		case rows := <-views:
			if got := rows[0].LastTradeTsNs; got != "1795721087978971514" {
				t.Fatalf("view %d: last_trade_ts_ns = %s, want the snapshot's", i, got)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("view %d never emitted", i)
		}
	}
}
