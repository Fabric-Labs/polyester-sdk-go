package decode_test

import (
	stderrors "errors"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// canceledUnfilled is a base-quantity order canceled with no fill, whose
// total the server took down to its filled quantity, 0. A self-trade
// prevention decrement and a modify to at or below the filled quantity
// reduce only an order sized in base quantity.
func canceledUnfilled(reason orderv1.CancelReason) *orderv1.Order {
	return &orderv1.Order{
		OrderId:      51,
		SymbolId:     3,
		Side:         orderv1.Side_BUY,
		Status:       orderv1.OrderStatus_CANCELED,
		OrderType:    orderv1.OrderType_LIMIT,
		TimeInForce:  orderv1.TimeInForce_GTC,
		PriceTicks:   5000,
		CreatedTsNs:  1_700_000_000_000_000_000,
		TerminalTsNs: 1_700_000_000_000_000_500,
		Version:      2,
		CancelReason: reason,
	}
}

func getOrder(t *testing.T, msg *orderv1.Order) models.Order {
	t.Helper()
	got, err := decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: msg})
	if err != nil {
		t.Fatalf("GetOrderFromProto: %v", err)
	}
	return *got.Order
}

// requireZeroQty fails unless qty is a decoded zero quantity: 0 scaled, in
// the order base domain, naming symbolID (nil for none).
func requireZeroQty(t *testing.T, name string, qty models.QtyScaled, symbolID *uint32) {
	t.Helper()
	if qty.Scaled() != 0 || qty.Domain() != models.QuantityDomainOrderBase {
		t.Fatalf("%s = %d in domain %q, want a zero order_base quantity", name, qty.Scaled(), qty.Domain())
	}
	got := qty.SymbolID()
	if (got == nil) != (symbolID == nil) || (got != nil && *got != *symbolID) {
		t.Fatalf("%s symbol ID = %v, want %v", name, got, symbolID)
	}
}

func TestOrderFromProtoReadsZeroOrigQtyAfterSTPDecrement(t *testing.T) {
	msg := canceledUnfilled(orderv1.CancelReason_CANCEL_REASON_STP_DECREMENT)
	if msg.SubmittedMaxQuoteDebitScaled != nil {
		t.Fatal("fixture must be a base-quantity order")
	}
	got := getOrder(t, msg)
	if got.CancelReason != "stp_decrement" {
		t.Fatalf("CancelReason = %q", got.CancelReason)
	}
	sid := uint32(3)
	requireZeroQty(t, "OrigQty", got.OrigQty, &sid)
	requireZeroQty(t, "CumQty", got.CumQty, &sid)
	requireZeroQty(t, "LeavesQty", got.LeavesQty, &sid)
}

func TestOrderFromProtoKeepsPositiveOrigQtyOfQuoteSizedBuyAfterSTPDecrement(t *testing.T) {
	// Self-trade prevention does not reduce a BUY sized in quote: its
	// original quantity stays above 0 with no fill.
	msg := canceledUnfilled(orderv1.CancelReason_CANCEL_REASON_STP_DECREMENT)
	msg.OrderType = orderv1.OrderType_MARKET
	msg.TimeInForce = orderv1.TimeInForce_IOC
	msg.OrigQtyScaled = 2940
	msg.SubmittedMaxQuoteDebitScaled = proto.Int64(6_000_000_000)
	got := getOrder(t, msg)
	if got.OrigQty.Scaled() != 2940 {
		t.Fatalf("OrigQty = %d, want 2940", got.OrigQty.Scaled())
	}
	sid := uint32(3)
	requireZeroQty(t, "CumQty", got.CumQty, &sid)
}

func TestOrderFromProtoReadsZeroOrigQtyAfterModifyToFilled(t *testing.T) {
	got := getOrder(t, canceledUnfilled(orderv1.CancelReason_CANCEL_REASON_QTY_AT_OR_BELOW_FILLED))
	if got.CancelReason != "qty_at_or_below_filled" {
		t.Fatalf("CancelReason = %q", got.CancelReason)
	}
	sid := uint32(3)
	requireZeroQty(t, "OrigQty", got.OrigQty, &sid)
	requireZeroQty(t, "CumQty", got.CumQty, &sid)
}

func TestOrderFromProtoReadsZeroOrigQtyOfRefusedBatchReplacement(t *testing.T) {
	// GetOrder of a batch replacement refused as it applied: no order terms,
	// 0 placeholders for its quantities and prices.
	got := getOrder(t, &orderv1.Order{
		OrderId:        52,
		Status:         orderv1.OrderStatus_REJECTED,
		RejectReason:   orderv1.ErrorCode_ERROR_CODE_MODIFY_APPLY_EXPIRED,
		BatchRequestId: 77,
		Version:        1,
		CreatedTsNs:    1_700_000_000_000_000_000,
	})
	if got.SymbolID != 0 {
		t.Fatalf("SymbolID = %d", got.SymbolID)
	}
	requireZeroQty(t, "OrigQty", got.OrigQty, nil)
	requireZeroQty(t, "CumQty", got.CumQty, nil)
}

// ConnectRPC JSON omits a quantity of 0, which proto3 reads as 0: an omitted
// origQtyScaled decodes as a zero quantity, as the binary encoding does.
func TestOrderFromProtoReadsOmittedJSONOrigQtyAsZero(t *testing.T) {
	const body = `{
		"orderId": "51",
		"symbolId": 3,
		"side": "BUY",
		"status": "CANCELED",
		"orderType": "LIMIT",
		"timeInForce": "GTC",
		"priceTicks": "5000",
		"createdTsNs": "1700000000000000000",
		"terminalTsNs": "1700000000000000500",
		"version": 2,
		"cancelReason": "CANCEL_REASON_STP_DECREMENT"
	}`
	var msg orderv1.Order
	if err := protojson.Unmarshal([]byte(body), &msg); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&msg, canceledUnfilled(orderv1.CancelReason_CANCEL_REASON_STP_DECREMENT)) {
		t.Fatalf("JSON fixture decodes to %v", &msg)
	}
	got := getOrder(t, &msg)
	sid := uint32(3)
	requireZeroQty(t, "OrigQty", got.OrigQty, &sid)
	requireZeroQty(t, "CumQty", got.CumQty, &sid)
	requireZeroQty(t, "LeavesQty", got.LeavesQty, &sid)
}

// A negative quantity is corrupt: the read fails with a
// ResponseContractError naming the field, rather than reading 0.
func TestGetOrderRefusesANegativeQuantity(t *testing.T) {
	for _, tc := range []struct {
		field string
		set   func(*orderv1.Order)
	}{
		{"Order.orig_qty_scaled", func(o *orderv1.Order) { o.OrigQtyScaled = -1 }},
		{"Order.cum_qty_scaled", func(o *orderv1.Order) { o.CumQtyScaled = -1 }},
		{"Order.leaves_qty_scaled", func(o *orderv1.Order) { o.LeavesQtyScaled = -1 }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			msg := &orderv1.Order{OrderId: 1, SymbolId: 3}
			tc.set(msg)
			_, err := decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: msg})
			var contract *sdkerrors.ResponseContractError
			if !stderrors.As(err, &contract) {
				t.Fatalf("err = %v, want *ResponseContractError", err)
			}
			if contract.Operation != tc.field || contract.Msg != "negative quantity -1" {
				t.Fatalf("err = %+v", contract)
			}
		})
	}
}
