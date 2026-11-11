package hardening_test

// POLY-9302 on top of POLY-9228: order, trade and trigger reads take their
// pair's quantity scale from the spot config alone. They are scaled while
// the funding (deposit/withdraw) config is still loading or after it failed,
// and never wait for it.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestL2OrderReadsScaleFromTheSpotConfigWithoutTheFundingConfig(t *testing.T) {
	order := &orderv1.Order{OrderId: 7, SymbolId: 1, OrigQtyScaled: 4_148_504, CumQtyScaled: 4_148_504}
	trigger := &triggersv1.Trigger{TriggerId: 11, SymbolId: 1, QtyScaled: 100_000_000}
	answers := map[string]proto.Message{
		"GetOrder":        &orderv1.GetOrderResponse{Order: order},
		"GetOpenOrders":   &orderv1.GetOpenOrdersResponse{Orders: []*orderv1.Order{order}},
		"GetOrderHistory": &orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{order}},
		"GetUserTrades":   &orderv1.GetUserTradesResponse{Trades: []*orderv1.UserTrade{{SymbolId: 1, MatchId: 9, QtyScaled: 4_148_504}}},
		"GetTrigger":      &triggersv1.GetTriggerResponse{Trigger: trigger},
	}
	for _, state := range []fundingState{fundingPending, fundingFailed} {
		t.Run(string(state), func(t *testing.T) {
			funding := make(chan struct{})
			t.Cleanup(func() { close(funding) })
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				_, _ = io.Copy(io.Discard, req.Body)
				rpc := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
				var answer proto.Message
				switch rpc {
				case "GetSpotConfig":
					answer = &marketdatav1.GetSpotConfigResponse{Pairs: []*marketdatav1.PairConfig{{
						Symbol: "BTC-USDT", SymbolId: 1, BaseAsset: "BTC", QuoteAsset: "USDT",
						BaseQuantityScale: 8, QuoteQuantityScale: 6, ReferencePriceScale: 9,
					}}}
				case "GetDepositWithdrawConfig":
					if state == fundingPending {
						select {
						case <-funding:
						case <-req.Context().Done():
						}
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"code":"internal","message":"funding down"}`))
					return
				default:
					var ok bool
					if answer, ok = answers[rpc]; !ok {
						http.NotFound(w, req)
						return
					}
				}
				body, err := proto.Marshal(answer)
				if err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/proto")
				_, _ = w.Write(body)
			}))
			t.Cleanup(srv.Close)
			kp := auth.GenerateEd25519Keypair()
			client, err := polyester.New(polyester.Config{
				APIURL:          srv.URL,
				WSURL:           "ws://127.0.0.1:9/connection/websocket",
				APIKeyID:        "ak_test",
				APIPrivateKey:   kp.SecretKeyHex,
				HydrateCatalogs: true,
				Timeout:         30 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if state == fundingFailed {
				if err := client.WaitForFundingCatalog(context.Background()); err == nil {
					t.Fatal("the funding config loaded")
				}
			}

			// Started right after the client: each read waits for the spot config only.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			got, err := client.Orders.Get(ctx, nil, models.OrderKeyByID("7"), nil, false, false)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			requireSpotScale(t, "Get CumQty", got.Order.CumQty)
			open, err := client.Orders.ListOpen(ctx, nil, nil, nil, nil, false, false, nil)
			if err != nil {
				t.Fatalf("ListOpen: %v", err)
			}
			requireSpotScale(t, "ListOpen CumQty", open.Orders[0].CumQty)
			history, err := client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 100, false, false, nil)
			if err != nil {
				t.Fatalf("ListHistory: %v", err)
			}
			requireSpotScale(t, "ListHistory CumQty", history.Orders[0].CumQty)
			trades, err := client.Trades.List(ctx, nil, nil, nil, nil, 100, nil, nil)
			if err != nil {
				t.Fatalf("Trades.List: %v", err)
			}
			requireSpotScale(t, "Trades.List Qty", trades.Trades[0].Qty)
			read, err := client.Triggers.Get(ctx, nil, "11", nil)
			if err != nil {
				t.Fatalf("Triggers.Get: %v", err)
			}
			requireSpotScale(t, "Triggers.Get Qty", read.Qty)
		})
	}
}

func requireSpotScale(t *testing.T, label string, q models.QtyScaled) {
	t.Helper()
	if scale := q.Scale(); scale == nil || *scale != 8 {
		t.Fatalf("%s scale = %v, want 8 from the spot config", label, scale)
	}
}
