package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

type tradesAfterHandler struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	mu       sync.Mutex
	requests []*marketdatav1.GetTradesRequest
}

func (h *tradesAfterHandler) GetSpotConfig(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{Symbol: "ETH-USDT", SymbolId: 2, BaseQuantityScale: 8}},
	}), nil
}

func (h *tradesAfterHandler) GetTrades(_ context.Context, req *connect.Request[marketdatav1.GetTradesRequest]) (*connect.Response[marketdatav1.GetTradesResponse], error) {
	h.mu.Lock()
	h.requests = append(h.requests, req.Msg)
	h.mu.Unlock()
	return connect.NewResponse(&marketdatav1.GetTradesResponse{
		Trades: []*marketdatav1.MarketTrade{
			{SymbolId: 2, MatchId: 1_099_511_627_777, IsBuy: true, PriceTicks: 2_000_000_000_000, QtyScaled: 50_000_000, TsNs: 10},
		},
		CompleteThroughTradeId: 1_099_511_627_790,
		NextAfterTradeId:       1_099_511_627_790,
		EpochEnds:              []*marketdatav1.TradeEpochEnd{{Epoch: 1, LastTradeId: 1_099_511_627_775}},
	}), nil
}

func TestL2GetTradesAfterSendsCursorAndDecodesBackfill(t *testing.T) {
	handler := &tradesAfterHandler{}
	mux := http.NewServeMux()
	path, h := marketdatav1connect.NewMarketDataServiceHandler(handler)
	mux.Handle(path, h)
	zPath, zH := chainzipperv1connect.NewZipperServiceHandler(&zipperHandler{fn: func(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
		return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{}), nil
	}})
	mux.Handle(zPath, zH)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL: srv.URL, APIKeyID: "ak_test", APIPrivateKey: kp.SecretKeyHex,
		HydrateCatalogs: true, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}

	symbol := "ETH-USDT"
	for _, bad := range []string{"", "abc", "-1"} {
		if _, err := client.MarketData.GetTradesAfter(context.Background(), &symbol, nil, bad, 100); err == nil {
			t.Fatalf("afterTradeID %q: want a validation error", bad)
		}
	}

	result, err := client.MarketData.GetTradesAfter(context.Background(), &symbol, nil, "0", 100)
	if err != nil {
		t.Fatal(err)
	}
	handler.mu.Lock()
	requests := handler.requests
	handler.mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("GetTrades calls=%d, want 1 (invalid cursors must not reach the server)", len(requests))
	}
	req := requests[0]
	if req.AfterTradeId == nil || req.GetAfterTradeId() != 0 || req.GetSymbolId() != 2 || req.GetLimit() != 100 {
		t.Fatalf("request %+v", req)
	}
	if req.GetPageToken() != "" || req.StartTime != nil || req.EndTime != nil || req.GetSide() != marketdatav1.SideFilter_SIDE_UNSPECIFIED {
		t.Fatalf("after_trade_id combined with other filters: %+v", req)
	}
	if len(result.Trades) != 1 || result.Trades[0].MatchID != "1099511627777" || result.Trades[0].Side != "buy" {
		t.Fatalf("trades %+v", result.Trades)
	}
	if result.CompleteThroughTradeID != "1099511627790" || result.NextAfterTradeID != "1099511627790" {
		t.Fatalf("cursors %q %q", result.CompleteThroughTradeID, result.NextAfterTradeID)
	}
	if !reflect.DeepEqual(result.EpochEnds, []models.TradeEpochEnd{{Epoch: 1, LastTradeID: "1099511627775"}}) {
		t.Fatalf("epoch ends %+v", result.EpochEnds)
	}
}
