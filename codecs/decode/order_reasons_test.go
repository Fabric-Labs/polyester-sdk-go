package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestOrderFromProtoDecodesEndReasons(t *testing.T) {
	cases := []struct {
		name   string
		msg    *orderv1.Order
		reject string
		cancel string
	}{
		{
			name:   "rejected",
			msg:    &orderv1.Order{OrderId: 1, Status: orderv1.OrderStatus_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_POST_ONLY_CROSS},
			reject: "POST_ONLY_CROSS",
		},
		{
			name:   "rejected with a code newer than the SDK",
			msg:    &orderv1.Order{OrderId: 2, Status: orderv1.OrderStatus_REJECTED, RejectReason: orderv1.ErrorCode(9999)},
			reject: "UNKNOWN_ERROR_CODE(9999)",
		},
		{
			name:   "rejected by a new trigger refusal",
			msg:    &orderv1.Order{OrderId: 3, Status: orderv1.OrderStatus_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_TWAP_CAPACITY_REACHED},
			reject: "TWAP_CAPACITY_REACHED",
		},
		{
			name:   "canceled",
			msg:    &orderv1.Order{OrderId: 4, Status: orderv1.OrderStatus_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_USER_REQUEST},
			cancel: "user_request",
		},
		{
			name:   "canceled at or below filled",
			msg:    &orderv1.Order{OrderId: 5, Status: orderv1.OrderStatus_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_QTY_AT_OR_BELOW_FILLED},
			cancel: "qty_at_or_below_filled",
		},
		{
			name:   "canceled with a reason newer than the SDK",
			msg:    &orderv1.Order{OrderId: 6, Status: orderv1.OrderStatus_CANCELED, CancelReason: orderv1.CancelReason(999)},
			cancel: "unknown(999)",
		},
		{
			name: "filled carries no reason",
			msg:  &orderv1.Order{OrderId: 7, Status: orderv1.OrderStatus_FILLED},
		},
		{
			// A server that predates the typed reasons sends neither field.
			name: "canceled without a reason",
			msg:  &orderv1.Order{OrderId: 8, Status: orderv1.OrderStatus_CANCELED},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decode.OrderFromProto(tc.msg)
			if got.RejectReason != tc.reject || got.CancelReason != tc.cancel {
				t.Fatalf("reject=%q cancel=%q, want reject=%q cancel=%q", got.RejectReason, got.CancelReason, tc.reject, tc.cancel)
			}
		})
	}
}

func TestOrderCancelReasonLabelNamesEveryGeneratedReason(t *testing.T) {
	if got := decode.OrderCancelReasonLabel(orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED); got != "" {
		t.Fatalf("unspecified label = %q", got)
	}
	for raw, name := range orderv1.CancelReason_name {
		if raw == 0 {
			continue
		}
		got := decode.OrderCancelReasonLabel(orderv1.CancelReason(raw))
		if got == "" || got[0] < 'a' || got[0] > 'z' || len(got) >= len(name) {
			t.Fatalf("%s label = %q", name, got)
		}
	}
}
