package decode_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// orderReadPaths decodes one order through every typed order read: GetOrder,
// open orders, order history and the private order stream.
func orderReadPaths(msg *orderv1.Order) map[string]models.Order {
	return map[string]models.Order{
		"GetOrder":     *decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: msg}).Order,
		"open orders":  decode.OrdersListFromOpen(&orderv1.GetOpenOrdersResponse{Orders: []*orderv1.Order{msg}}).Orders[0],
		"history":      decode.OrdersListFromHistory(&orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{msg}}).Orders[0],
		"order stream": decode.OrdersFromProto([]*orderv1.Order{msg})[0],
	}
}

func orderJSON(t *testing.T, order models.Order) map[string]any {
	t.Helper()
	raw, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func uint64Ptr(v uint64) *uint64 { return &v }

// Order reads keep the terminal time, origin and MARKET slippage protection
// the server sends, and omit each one the server leaves unset.
func TestOrderReadsKeepTerminalTimeOriginAndMarketSlippage(t *testing.T) {
	cases := []struct {
		name string
		msg  *orderv1.Order
		want map[string]any
	}{
		{
			name: "filled TWAP child with bps slippage",
			msg: &orderv1.Order{
				OrderId: 11, SymbolId: 1, Status: orderv1.OrderStatus_FILLED,
				OrderType:    orderv1.OrderType_MARKET,
				CreatedTsNs:  1_700_000_000_000_000_000,
				TerminalTsNs: 1_700_000_000_250_000_000,
				Origin: &orderv1.OrderOrigin{
					Scope:       orderv1.OrderOriginScope_STANDALONE_TRIGGER,
					TriggerType: orderv1.OrderTriggerType_TWAP,
					TriggerId:   uint64Ptr(77),
					ChildSeq:    3,
				},
				MarketMaxSlippageBps: 100,
			},
			want: map[string]any{
				"terminal_ts_ns": "1700000000250000000",
				"origin": map[string]any{
					"scope":        "standalone_trigger",
					"trigger_type": "twap",
					"trigger_id":   codecs.FormatUint64ID(77),
					"child_seq":    float64(3),
				},
				"market_max_slippage_bps": float64(100),
			},
		},
		{
			name: "attached-risk market child with a tick cap",
			msg: &orderv1.Order{
				OrderId: 12, SymbolId: 1, Status: orderv1.OrderStatus_CANCELED,
				OrderType:    orderv1.OrderType_MARKET,
				TerminalTsNs: 1_700_000_001_000_000_000,
				Origin: &orderv1.OrderOrigin{
					Scope:         orderv1.OrderOriginScope_ATTACHED_RISK,
					TriggerType:   orderv1.OrderTriggerType_STOP_LOSS,
					TriggerId:     uint64Ptr(78),
					ParentOrderId: uint64Ptr(10),
				},
				MarketClientRefPriceTicks: 50_000_000_000,
				MarketMaxSlippageTicks:    2_500_000,
			},
			want: map[string]any{
				"terminal_ts_ns": "1700000001000000000",
				"origin": map[string]any{
					"scope":           "attached_risk",
					"trigger_type":    "stop_loss",
					"trigger_id":      codecs.FormatUint64ID(78),
					"parent_order_id": codecs.FormatUint64ID(10),
				},
				"market_client_ref_price": map[string]any{"ticks": float64(50_000_000_000)},
				"market_max_slippage":     map[string]any{"ticks": float64(2_500_000)},
			},
		},
		{
			name: "direct order",
			msg: &orderv1.Order{
				OrderId: 13, SymbolId: 1, Status: orderv1.OrderStatus_FILLED,
				TerminalTsNs: 1_700_000_002_000_000_000,
				Origin:       &orderv1.OrderOrigin{Scope: orderv1.OrderOriginScope_DIRECT},
			},
			want: map[string]any{
				"terminal_ts_ns": "1700000002000000000",
				"origin":         map[string]any{"scope": "direct"},
			},
		},
		{
			name: "open order without any of them",
			msg:  &orderv1.Order{OrderId: 14, SymbolId: 1, Status: orderv1.OrderStatus_WORKING},
			want: map[string]any{},
		},
	}
	keys := []string{
		"terminal_ts_ns", "origin", "market_client_ref_price", "market_max_slippage", "market_max_slippage_bps",
	}
	for _, tc := range cases {
		for path, order := range orderReadPaths(tc.msg) {
			got := orderJSON(t, order)
			for _, key := range keys {
				want, wantOK := tc.want[key]
				value, ok := got[key]
				if ok != wantOK || !reflect.DeepEqual(value, want) {
					t.Errorf("%s via %s: %s=%v (present %v), want %v (present %v)", tc.name, path, key, value, ok, want, wantOK)
				}
			}
		}
	}
}
