package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func orderEvent(event *privatewsv1.OrderEvent) *privatewsv1.UnitEvent {
	return &privatewsv1.UnitEvent{Kind: &privatewsv1.UnitEvent_Order{Order: event}}
}

func TestOrderEventReasons(t *testing.T) {
	events := decode.OrderUnitEventsFromProto([]*privatewsv1.UnitEvent{
		orderEvent(&privatewsv1.OrderEvent{OrderId: 1, Kind: privatewsv1.OrderEvent_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_POST_ONLY_CROSS}),
		orderEvent(&privatewsv1.OrderEvent{OrderId: 2, Kind: privatewsv1.OrderEvent_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_USER_REQUEST}),
		orderEvent(&privatewsv1.OrderEvent{OrderId: 3, Kind: privatewsv1.OrderEvent_REPLACED, CancelReason: orderv1.CancelReason_CANCEL_REASON_REPLACED, ReplacementOrderId: 4}),
		orderEvent(&privatewsv1.OrderEvent{OrderId: 5, Kind: privatewsv1.OrderEvent_ACCEPTED}),
		orderEvent(&privatewsv1.OrderEvent{OrderId: 6, Kind: privatewsv1.OrderEvent_CANCELED, CancelReason: orderv1.CancelReason(999)}),
	}, nil)
	want := []struct {
		kind   models.OrderEventKind
		reject string
		cancel string
	}{
		{models.OrderEventRejected, "POST_ONLY_CROSS", ""},
		{models.OrderEventCanceled, "", "USER_REQUEST"},
		{models.OrderEventReplaced, "", "REPLACED"},
		{models.OrderEventAccepted, "", ""},
		{models.OrderEventCanceled, "", "UNKNOWN_CANCEL_REASON(999)"},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d", len(events), len(want))
	}
	for i, w := range want {
		got := events[i].Order
		if got == nil || got.Kind != w.kind || got.RejectReason != w.reject || got.CancelReason != w.cancel {
			t.Fatalf("event %d = %+v, want kind=%s reject=%q cancel=%q", i, got, w.kind, w.reject, w.cancel)
		}
	}
}
