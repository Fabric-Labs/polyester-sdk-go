package services

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	triggers "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// orderReadScalesHandler answers order, trade and trigger reads with one
// BTC-USDT (symbol_id 1) row each, quantities at base_quantity_scale 8.
type orderReadScalesHandler struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	triggersv1connect.UnimplementedTriggersServiceHandler
}

func scalesOrder() *orders.Order {
	return &orders.Order{
		OrderId: 7, SymbolId: 1, Side: orders.Side_BUY, Status: orders.OrderStatus_FILLED,
		OrderType: orders.OrderType_MARKET, TimeInForce: orders.TimeInForce_IOC,
		OrigQtyScaled: 8000, CumQtyScaled: 8000, LeavesQtyScaled: 0,
		FeeTotal: &orders.OrderFeeTotal{QtyScaled: 8000},
	}
}

func scalesTrade() *orders.UserTrade {
	return &orders.UserTrade{MatchId: 3, OrderId: 7, SymbolId: 1, QtyScaled: 8000, PriceTicks: 1}
}

func scalesTrigger() *triggers.Trigger {
	return &triggers.Trigger{
		TriggerId: 9, SymbolId: 1, QtyScaled: 8000,
		RuntimeDetails: &triggers.Trigger_TwapState{TwapState: &triggers.TwapDetails{ExecutedQtyScaled: 4000}},
	}
}

func (orderReadScalesHandler) GetOrder(context.Context, *connect.Request[orders.GetOrderRequest]) (*connect.Response[orders.GetOrderResponse], error) {
	return connect.NewResponse(&orders.GetOrderResponse{Order: scalesOrder(), Trades: []*orders.UserTrade{scalesTrade()}}), nil
}
func (orderReadScalesHandler) GetOpenOrders(context.Context, *connect.Request[orders.GetOpenOrdersRequest]) (*connect.Response[orders.GetOpenOrdersResponse], error) {
	return connect.NewResponse(&orders.GetOpenOrdersResponse{Orders: []*orders.Order{scalesOrder()}}), nil
}
func (orderReadScalesHandler) GetOrderHistory(context.Context, *connect.Request[orders.GetOrderHistoryRequest]) (*connect.Response[orders.GetOrderHistoryResponse], error) {
	return connect.NewResponse(&orders.GetOrderHistoryResponse{Orders: []*orders.Order{scalesOrder()}}), nil
}
func (orderReadScalesHandler) GetUserTrades(context.Context, *connect.Request[orders.GetUserTradesRequest]) (*connect.Response[orders.GetUserTradesResponse], error) {
	return connect.NewResponse(&orders.GetUserTradesResponse{Trades: []*orders.UserTrade{scalesTrade()}}), nil
}
func (orderReadScalesHandler) GetTrigger(context.Context, *connect.Request[triggers.GetTriggerRequest]) (*connect.Response[triggers.GetTriggerResponse], error) {
	return connect.NewResponse(&triggers.GetTriggerResponse{Trigger: scalesTrigger()}), nil
}
func (orderReadScalesHandler) ListTriggers(context.Context, *connect.Request[triggers.ListTriggersRequest]) (*connect.Response[triggers.ListTriggersResponse], error) {
	return connect.NewResponse(&triggers.ListTriggersResponse{Triggers: []*triggers.Trigger{scalesTrigger()}}), nil
}

func newOrderReadScalesFactory(t *testing.T, wire string) *transport.Factory {
	t.Helper()
	mux := http.NewServeMux()
	h := orderReadScalesHandler{}
	path, handler := ordersv1connect.NewOrdersReadServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = triggersv1connect.NewTriggersServiceHandler(h)
	mux.Handle(path, handler)
	server := hardening.NewTestServer(mux)
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	return transport.NewFactory(cfg, creds, nil)
}

// requireQtyScale checks a read quantity's scale: 8 from the loaded catalog,
// or none without one.
func requireQtyScale(t *testing.T, label string, q models.QtyScaled, want *int) {
	t.Helper()
	got := q.Scale()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s scale = %d, want none without catalogs", label, *got)
	case want != nil && got == nil:
		t.Errorf("%s scale = none, want %d from the catalog", label, *want)
	case want != nil && *got != *want:
		t.Errorf("%s scale = %d, want %d", label, *got, *want)
	}
}

func requireOrderScales(t *testing.T, label string, o models.Order, want *int) {
	t.Helper()
	requireQtyScale(t, label+".OrigQty", o.OrigQty, want)
	requireQtyScale(t, label+".CumQty", o.CumQty, want)
	requireQtyScale(t, label+".InheritedCumQty", o.InheritedCumQty, want)
	requireQtyScale(t, label+".LeavesQty", o.LeavesQty, want)
	if o.FeeTotal == nil {
		t.Fatalf("%s.FeeTotal missing", label)
	}
	requireQtyScale(t, label+".FeeTotal.Qty", o.FeeTotal.Qty, want)
	if want != nil {
		if got, err := o.CumQty.Format(); err != nil || got != "0.00008" {
			t.Errorf("%s.CumQty.Format() = %q, %v; want 0.00008", label, got, err)
		}
	}
}

// POLY-9302: with the catalogs loaded, every order, trade and trigger read
// carries the pair's base quantity scale, as create and preview do; without
// them the quantities stay unscaled.
func TestPoly9302OrderReadsCarryCatalogQuantityScale(t *testing.T) {
	eight := 8
	for _, wire := range []string{"binary", "json"} {
		for _, loaded := range []bool{true, false} {
			name := wire + "/catalogs"
			want := &eight
			if !loaded {
				name, want = wire+"/no-catalogs", nil
			}
			t.Run(name, func(t *testing.T) {
				factory := newOrderReadScalesFactory(t, wire)
				cats := catalogs.NewManager()
				if loaded {
					if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)}}}); err != nil {
						t.Fatal(err)
					}
				}
				ctx := context.Background()
				svc := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
				got, err := svc.Get(ctx, nil, models.OrderKeyByID("7"), nil, false, false)
				if err != nil {
					t.Fatal(err)
				}
				requireOrderScales(t, "Get", *got.Order, want)
				requireQtyScale(t, "Get.Trades[0].Qty", got.Trades[0].Qty, want)
				open, err := svc.ListOpen(ctx, nil, nil, nil, nil, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				requireOrderScales(t, "ListOpen", open.Orders[0], want)
				history, err := svc.ListHistory(ctx, nil, nil, nil, nil, nil, 10, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				requireOrderScales(t, "ListHistory", history.Orders[0], want)
				trades, err := NewTradesService(factory, cats, nil, nil, nil).List(ctx, nil, nil, nil, nil, 10, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				requireQtyScale(t, "Trades.List[0].Qty", trades.Trades[0].Qty, want)
				triggerSvc := NewTriggersService(factory, cats, nil, nil, nil, nil, nil)
				trigger, err := triggerSvc.Get(ctx, nil, "9", nil)
				if err != nil {
					t.Fatal(err)
				}
				requireQtyScale(t, "Triggers.Get.Qty", trigger.Qty, want)
				requireQtyScale(t, "Triggers.Get.Details.ExecutedQty", trigger.Details.ExecutedQty, want)
				list, err := triggerSvc.List(ctx, nil, nil, nil, nil, 10, nil)
				if err != nil {
					t.Fatal(err)
				}
				requireQtyScale(t, "Triggers.List.Qty", list.Triggers[0].Qty, want)
				requireQtyScale(t, "Triggers.List.Details.ExecutedQty", list.Triggers[0].Details.ExecutedQty, want)
			})
		}
	}
}
