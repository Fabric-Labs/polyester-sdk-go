package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestOrderFromProtoMapsEnumsAndIDs(t *testing.T) {
	msg := &orderv1.Order{
		OrderId:         42,
		SymbolId:        3,
		ClientOrderId:   "coid-1",
		Side:            orderv1.Side_BUY,
		Status:          orderv1.OrderStatus_WORKING,
		OrderType:       orderv1.OrderType_LIMIT,
		TimeInForce:     orderv1.TimeInForce_GTC,
		OrigQtyScaled:   100,
		CumQtyScaled:    10,
		LeavesQtyScaled: 90,
		PriceTicks:      5000,
		AvgPriceTicks:   4990,
		CreatedTsNs:     1_700_000_000_000,
	}
	order := decode.OrderFromProto(msg)
	if order.OrderID != codecs.FormatUint64ID(42) {
		t.Fatalf("order_id=%q", order.OrderID)
	}
	if order.Side != "buy" || order.Status != "working" || order.OrderType != "limit" || order.TIF != "gtc" {
		t.Fatalf("order=%+v", order)
	}
	if order.OrigQty.Scaled != 100 {
		t.Fatalf("orig_qty=%+v", order.OrigQty)
	}
	msg.StateRevision = 7
	order = decode.OrderFromProto(msg)
	if order.StateRevision != 7 {
		t.Fatalf("state_revision=%d", order.StateRevision)
	}
}

func TestOrdersListFromProto(t *testing.T) {
	msg := &orderv1.GetOpenOrdersResponse{
		Orders:        []*orderv1.Order{{OrderId: 1, SymbolId: 1, Side: orderv1.Side_SELL}},
		NextPageToken: "tok",
	}
	result := decode.OrdersListFromOpen(msg)
	if len(result.Orders) != 1 || result.NextPageToken != "tok" {
		t.Fatalf("result=%+v", result)
	}
}

func TestGetOrderFromProtoIncludesTrades(t *testing.T) {
	msg := &orderv1.GetOrderResponse{
		Order: &orderv1.Order{OrderId: 7, SymbolId: 2},
		Trades: []*orderv1.UserTrade{{
			SymbolId: 2, MatchId: 99, OrderId: 7, Side: orderv1.Side_BUY,
		}},
	}
	result := decode.GetOrderFromProto(msg)
	if result.Order == nil || result.Order.OrderID != codecs.FormatUint64ID(7) {
		t.Fatalf("order=%+v", result.Order)
	}
	if len(result.Trades) != 1 || result.Trades[0].MatchID != "99" {
		t.Fatalf("trades=%+v", result.Trades)
	}
}

func TestModifyOrderFromProtoActionTakenEnum(t *testing.T) {
	msg := &orderv1.ModifyOrderResponse{
		ActionTaken:  orderv1.ModifyActionTaken_AMENDED,
		OldOrderId:    10,
		FinalOrderId:  11,
		Code:          "ok",
	}
	result := decode.ModifyOrderFromProto(msg)
	if result.ActionTaken != "amended" {
		t.Fatalf("action_taken=%q", result.ActionTaken)
	}
	if result.OldOrderID != codecs.FormatUint64ID(10) || result.FinalOrderID != codecs.FormatUint64ID(11) {
		t.Fatalf("result=%+v", result)
	}
}

func TestOrderMutationFromProtoCreateIncludesClientOrderID(t *testing.T) {
	msg := &orderv1.CreateOrderResponse{
		Status:        "accepted",
		OrderId:       42,
		ClientOrderId: "coid-1",
	}
	result := decode.OrderMutationFromProto(msg)
	if result.Status != "accepted" || result.OrderID != codecs.FormatUint64ID(42) || result.ClientOrderID != "coid-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestOrderMutationFromProtoCancelOmitsClientOrderID(t *testing.T) {
	msg := &orderv1.CancelOrderResponse{Status: "cancelled", OrderId: 42}
	result := decode.OrderMutationFromCancel(msg)
	if result.Status != "cancelled" || result.OrderID != codecs.FormatUint64ID(42) || result.ClientOrderID != "" {
		t.Fatalf("result=%+v", result)
	}
}
