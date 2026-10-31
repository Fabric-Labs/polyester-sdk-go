package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1/marketoverviewv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

type overviewSnapshotHandler struct {
	marketoverviewv1connect.UnimplementedMarketOverviewServiceHandler
}

func (overviewSnapshotHandler) ListMarketOverview(_ context.Context, req *connect.Request[marketoverviewv1.ListMarketOverviewRequest]) (*connect.Response[marketoverviewv1.ListMarketOverviewResponse], error) {
	markets := make([]*marketoverviewv1.MarketOverview, 0, len(req.Msg.GetSymbolId()))
	for _, id := range req.Msg.GetSymbolId() {
		markets = append(markets, &marketoverviewv1.MarketOverview{SymbolId: id, LastPriceTicks: 1_000_000_000})
	}
	return connect.NewResponse(&marketoverviewv1.ListMarketOverviewResponse{Markets: markets}), nil
}

func TestL2FilteredMarketOverviewSubscriptionDropsUnrelatedSymbols(t *testing.T) {
	pubs := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(ws.Close)
	mux := http.NewServeMux()
	path, handler := marketoverviewv1connect.NewMarketOverviewServiceHandler(overviewSnapshotHandler{})
	mux.Handle(path, handler)
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)

	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
		map[string]any{"symbol": "ETH-USDT", "symbol_id": float64(2), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT(ws.WSURL(), api.URL, nil, 5*time.Second)
	t.Cleanup(rt.Close)
	service := services.NewMarketOverviewService(transport.NewFactory(cfg, nil, nil), cats, rt)

	sub, err := service.CreateSubscription(context.Background(), services.MarketOverviewCreateSubscriptionOptions{
		Symbols: []string{"BTC-USDT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	publish := func(markets ...*marketoverviewv1.MarketOverview) {
		payload, err := proto.Marshal(&marketoverviewv1.MarketOverviewBatch{Markets: markets})
		if err != nil {
			t.Fatal(err)
		}
		pubs <- payload
	}
	publish(&marketoverviewv1.MarketOverview{SymbolId: 2, LastPriceTicks: 5})
	publish(
		&marketoverviewv1.MarketOverview{SymbolId: 2, LastPriceTicks: 6},
		&marketoverviewv1.MarketOverview{SymbolId: 1, LastPriceTicks: 2_000_000_000},
	)

	deadline := time.After(5 * time.Second)
	for {
		var rows []models.MarketOverviewEntry
		select {
		case rows = <-sub.Updates():
		case <-deadline:
			t.Fatalf("no BTC-USDT update: %v", sub.Err())
		}
		for _, row := range rows {
			if row.SymbolID != 1 {
				t.Fatalf("filtered subscription emitted symbol %d: %+v", row.SymbolID, rows)
			}
		}
		if len(rows) == 1 && rows[0].LastPrice.Ticks() == 2_000_000_000 {
			return
		}
	}
}
