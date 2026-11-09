package services

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

const (
	trailingLimitSellOrderID = 7001
	trailingLimitBuyOrderID  = 7002
	// 85000 quote units, scaled by 1e9, as GetOrder returns it.
	trailingLimitTicks = int64(85_000_000_000_000)
)

// A SELL parent whose attached trailing stop buys back at most 85000, and a
// BUY parent whose trailing stop sells and takes no limit, as the order reads
// return them with attached risk.
func trailingLimitOrders() []*orderv1.Order {
	trailing := func(id uint64, limit int64) *orderv1.AttachedRisk {
		trigger := id + 100
		return &orderv1.AttachedRisk{TrailingStop: &orderv1.AttachedRiskTrailingStop{
			Policy: &orderv1.TrailingStopPolicy{
				TrailingDistance:   &orderv1.TrailingStopPolicy_TrailingDistanceBps{TrailingDistanceBps: 100},
				TriggerPriceSource: orderv1.TriggerPriceSource_LAST_PRICE,
				LimitPriceTicks:    limit,
			},
			State: &orderv1.AttachedRiskLegState{Status: orderv1.AttachedRiskLegState_CREATED, TriggerId: &trigger},
		}}
	}
	return []*orderv1.Order{
		{OrderId: trailingLimitSellOrderID, SymbolId: 1, Side: orderv1.Side_SELL, Status: orderv1.OrderStatus_WORKING, PriceTicks: 80_000_000_000_000, AttachedRisk: trailing(trailingLimitSellOrderID, trailingLimitTicks)},
		{OrderId: trailingLimitBuyOrderID, SymbolId: 1, Side: orderv1.Side_BUY, Status: orderv1.OrderStatus_WORKING, PriceTicks: 60_000_000_000_000, AttachedRisk: trailing(trailingLimitBuyOrderID, 0)},
	}
}

type trailingLimitReadHandler struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
}

func (trailingLimitReadHandler) GetOrder(_ context.Context, r *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	for _, order := range trailingLimitOrders() {
		if order.GetOrderId() == r.Msg.GetOrderId() {
			return connect.NewResponse(&orderv1.GetOrderResponse{Order: order}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, nil)
}

func (trailingLimitReadHandler) GetOpenOrders(context.Context, *connect.Request[orderv1.GetOpenOrdersRequest]) (*connect.Response[orderv1.GetOpenOrdersResponse], error) {
	return connect.NewResponse(&orderv1.GetOpenOrdersResponse{Orders: trailingLimitOrders()}), nil
}

func (trailingLimitReadHandler) GetOrderHistory(context.Context, *connect.Request[orderv1.GetOrderHistoryRequest]) (*connect.Response[orderv1.GetOrderHistoryResponse], error) {
	return connect.NewResponse(&orderv1.GetOrderHistoryResponse{Orders: trailingLimitOrders()}), nil
}

func trailingLimitReadService(t *testing.T, wire string) *OrdersService {
	t.Helper()
	_, h := ordersv1connect.NewOrdersReadServiceHandler(trailingLimitReadHandler{})
	srv := hardening.NewTestServer(h)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	return NewOrdersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil)
}

// trailingStopJSON is the attached trailing stop of the order with id, as a
// caller marshalling the read model sees it.
func trailingStopJSON(t *testing.T, orders []models.Order, id uint64) map[string]any {
	t.Helper()
	for _, order := range orders {
		if order.OrderID != codecs.FormatUint64ID(id) {
			continue
		}
		if order.AttachedRisk == nil || order.AttachedRisk.TrailingStop == nil {
			t.Fatalf("order %d lost its trailing stop: %+v", id, order.AttachedRisk)
		}
		raw, err := json.Marshal(order.AttachedRisk.TrailingStop)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Fatalf("order %d missing from %+v", id, orders)
	return nil
}

func assertTrailingLimitRead(t *testing.T, orders []models.Order) {
	t.Helper()
	unchanged := func(id uint64) map[string]any {
		return map[string]any{
			"distance_bps":         float64(100),
			"activation_price":     map[string]any{"ticks": float64(0)},
			"trigger_price_source": "last",
			"state":                map[string]any{"status": "created", "trigger_id": codecs.FormatUint64ID(id + 100)},
		}
	}

	sell := trailingStopJSON(t, orders, trailingLimitSellOrderID)
	if got, want := sell["limit_price"], (map[string]any{"ticks": float64(trailingLimitTicks)}); !reflect.DeepEqual(got, want) {
		t.Fatalf("sell parent's trailing stop limit_price=%v, want %v (85000); trailing stop: %v", got, want, sell)
	}
	delete(sell, "limit_price")
	if want := unchanged(trailingLimitSellOrderID); !reflect.DeepEqual(sell, want) {
		t.Fatalf("sell parent's other trailing-stop fields changed:\n got %v\nwant %v", sell, want)
	}

	buy := trailingStopJSON(t, orders, trailingLimitBuyOrderID)
	if limit, ok := buy["limit_price"]; ok {
		t.Fatalf("buy parent's trailing stop has no limit, but reads limit_price=%v", limit)
	}
	if want := unchanged(trailingLimitBuyOrderID); !reflect.DeepEqual(buy, want) {
		t.Fatalf("buy parent's trailing-stop fields changed:\n got %v\nwant %v", buy, want)
	}
}

// Every order read and the order stream expose the buy-back limit that
// GetOrder returns on an attached trailing stop, exactly, and leave it unset
// when the server sends none.
func TestOrderReadsExposeAttachedTrailingStopLimitPrice(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire+"/get", func(t *testing.T) {
			service := trailingLimitReadService(t, wire)
			var orders []models.Order
			for _, id := range []uint64{trailingLimitSellOrderID, trailingLimitBuyOrderID} {
				got, err := service.Get(context.Background(), nil, models.OrderKeyByID(codecs.FormatUint64ID(id)), nil, true, true)
				if err != nil {
					t.Fatal(err)
				}
				orders = append(orders, *got.Order)
			}
			assertTrailingLimitRead(t, orders)
		})
		t.Run(wire+"/list_open", func(t *testing.T) {
			got, err := trailingLimitReadService(t, wire).ListOpen(context.Background(), nil, nil, nil, nil, true, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertTrailingLimitRead(t, got.Orders)
		})
		t.Run(wire+"/list_history", func(t *testing.T) {
			symbolID := uint32(1)
			got, err := trailingLimitReadService(t, wire).ListHistory(context.Background(), nil, nil, nil, &symbolID, nil, 50, true, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertTrailingLimitRead(t, got.Orders)
		})
	}
	// Orders.Subscribe decodes its snapshots and updates with this decoder.
	t.Run("stream", func(t *testing.T) {
		assertTrailingLimitRead(t, decode.OrdersFromProto(trailingLimitOrders()))
	})
}
