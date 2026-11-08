package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func marketDataCatalogs(t *testing.T) *catalogs.Manager {
	t.Helper()
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{
		"assets": []any{
			map[string]any{"asset": "BTC", "quantityScale": float64(8), "marketDataVolumeScale": float64(8)},
			map[string]any{"asset": "BONK", "quantityScale": float64(0), "marketDataVolumeScale": float64(0)},
		},
		"pairs": []any{
			map[string]any{
				"symbol": "BTC-USDT", "symbolId": float64(1), "baseAsset": "BTC", "quoteAsset": "USDT", "baseQuantityScale": float64(8),
				"marketdata": map[string]any{"orderbookPriceBuckets": []any{0.01, 0.1, float64(1), float64(10), float64(100)}},
			},
			map[string]any{
				"symbol": "BONK-USDT", "symbolId": float64(7), "baseAsset": "BONK", "quoteAsset": "USDT", "baseQuantityScale": float64(0),
				"marketdata": map[string]any{"orderbookPriceBuckets": []any{1e-9, 1e-8, 1e-7, 1e-6, 1e-5}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return cats
}

// POLY-8785 parity: Go keeps every digit of sub-micro price buckets.
func TestOrderbookPriceBucketsKeepSubMicroIncrements(t *testing.T) {
	cats := marketDataCatalogs(t)
	for symbol, want := range map[string][]string{
		"BONK-USDT": {"0.000000001", "0.00000001", "0.0000001", "0.000001", "0.00001"},
		"BTC-USDT":  {"0.01", "0.1", "1", "10", "100"},
	} {
		got := cats.OrderbookPriceBucketsForSymbol(symbol)
		if len(got) != len(want) {
			t.Fatalf("%s: %q, want %q", symbol, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: %q, want %q", symbol, got, want)
			}
		}
	}
}

// POLY-8783 parity: Go decodes live overview rows with the catalogs, so they
// keep the symbol and formatted base volume list rows have.
func TestLiveOverviewRowsAreEnrichedFromTheCatalogs(t *testing.T) {
	volume := int64(250)
	payload, err := proto.Marshal(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, Volume_24HBaseScaled: &volume}}})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := decode.MarketOverviewBatchDecoder(marketDataCatalogs(t))(payload)
	if err != nil {
		t.Fatal(err)
	}
	row := batch.Markets[0]
	if row.Symbol != "BTC-USDT" || row.Volume24HBase == nil || *row.Volume24HBase != "0.0000025" {
		t.Fatalf("live row %+v", row)
	}
}

func overviewSparklineSnapshot() *overviewpb.ListMarketOverviewResponse {
	volume := int64(250)
	return &overviewpb.ListMarketOverviewResponse{Markets: []*overviewpb.MarketOverview{{
		SymbolId: 1, LastPriceTicks: 101, Change_24HBps: -14, Volume_24HBaseScaled: &volume,
		Sparklines: []*overviewpb.Sparkline{
			{Interval: overviewpb.SparklineInterval_SPARKLINE_24H, CloseTicks: []int64{101, 100, 99}},
			{Interval: overviewpb.SparklineInterval_SPARKLINE_1H},
		},
	}}}
}

func sparklineTicks(row models.MarketOverviewEntry) string {
	parts := make([]string, 0, len(row.Sparklines))
	for _, s := range row.Sparklines {
		ticks := make([]string, 0, len(s.ClosePrices))
		for _, p := range s.ClosePrices {
			ticks = append(ticks, fmt.Sprint(p.Ticks()))
		}
		parts = append(parts, s.Interval+"="+strings.Join(ticks, ","))
	}
	return strings.Join(parts, " ")
}

const wantSparklines = "SPARKLINE_24H=101,100,99 SPARKLINE_1H="

// POLY-8789 and POLY-8786 parity: List exposes the requested sparklines and the
// 24-hour change.
func TestOverviewListExposesRequestedSparklinesAndThe24hChange(t *testing.T) {
	var mu sync.Mutex
	var sent []bool
	server := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req overviewpb.ListMarketOverviewRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Error(err)
		}
		mu.Lock()
		sent = append(sent, req.GetIncludeSparklines())
		mu.Unlock()
		response := overviewSparklineSnapshot()
		if !req.GetIncludeSparklines() {
			response.Markets[0].Sparklines = nil
		}
		w.Header().Set("Content-Type", "application/proto")
		out, _ := proto.Marshal(response)
		_, _ = w.Write(out)
	}))
	defer server.Close()
	factory := transport.NewFactory(transport.Config{APIURL: server.URL, Timeout: 10 * time.Second}, nil, nil)
	service := NewMarketOverviewService(factory, marketDataCatalogs(t), nil)
	with, err := service.List(context.Background(), nil, 50, "", true)
	if err != nil {
		t.Fatal(err)
	}
	without, err := service.List(context.Background(), nil, 50, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(sent) != "[true false]" {
		t.Fatalf("include_sparklines sent %v", sent)
	}
	if got := sparklineTicks(with.Markets[0]); got != wantSparklines {
		t.Fatalf("sparklines %q, want %q", got, wantSparklines)
	}
	if with.Markets[0].Change24hBps != -14 || without.Markets[0].Change24hBps != -14 {
		t.Fatalf("change %d / %d", with.Markets[0].Change24hBps, without.Markets[0].Change24hBps)
	}
	if len(without.Markets[0].Sparklines) != 0 {
		t.Fatalf("unrequested sparklines %+v", without.Markets[0].Sparklines)
	}
}

// POLY-8789: live publications carry no sparklines; a managed subscription
// keeps the snapshot's when a live update replaces the row.
func TestManagedOverviewKeepsSnapshotSparklinesOnLiveUpdates(t *testing.T) {
	live := make(chan []byte, 1)
	ws := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for id := uint32(1); id <= 2; id++ {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
			if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(id)); err != nil {
				return
			}
		}
		if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoPublication(<-live)); err != nil {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer ws.Close()
	api := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		out, _ := proto.Marshal(overviewSparklineSnapshot())
		_, _ = w.Write(out)
	}))
	defer api.Close()
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), api.URL, nil, nil, 200)
	defer rt.Close()
	factory := transport.NewFactory(transport.Config{APIURL: api.URL, Timeout: 10 * time.Second}, nil, nil)
	service := NewMarketOverviewService(factory, marketDataCatalogs(t), rt)
	events := make(chan []models.MarketOverviewEntry, 4)
	sub, err := service.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		IncludeSparklines: true,
		OnEvent:           func(rows []models.MarketOverviewEntry) { events <- rows },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	next := func() models.MarketOverviewEntry {
		t.Helper()
		select {
		case rows := <-events:
			return rows[0]
		case <-time.After(10 * time.Second):
			t.Fatal("no overview event")
			return models.MarketOverviewEntry{}
		}
	}
	if row := next(); sparklineTicks(row) != wantSparklines {
		t.Fatalf("snapshot sparklines %q", sparklineTicks(row))
	}
	payload, _ := proto.Marshal(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, LastPriceTicks: 102, Change_24HBps: 7}}})
	live <- payload
	row := next()
	if row.LastPrice.Ticks() != 102 || row.Change24hBps != 7 || row.Symbol != "BTC-USDT" {
		t.Fatalf("live row %+v", row)
	}
	if got := sparklineTicks(row); got != wantSparklines {
		t.Fatalf("live sparklines %q, want %q", got, wantSparklines)
	}
}
