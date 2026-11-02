package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// A market halt ends orders with CANCEL_REASON_MARKET_HALTED (17) and
// triggers with TRIGGER_CANCEL_REASON_MARKET_HALTED (13). Both read as
// market_halted wherever the reasons appear.
func TestMarketHaltReasonsLabelOnReadsAndStreams(t *testing.T) {
	if orderv1.CancelReason_CANCEL_REASON_MARKET_HALTED != 17 ||
		triggersv1.TriggerCancelReason_TRIGGER_CANCEL_REASON_MARKET_HALTED != 13 {
		t.Fatal("generated market-halt reason numbers changed")
	}

	order := decode.OrderFromProto(&orderv1.Order{
		OrderId: 1, Status: orderv1.OrderStatus_CANCELED,
		CancelReason: orderv1.CancelReason_CANCEL_REASON_MARKET_HALTED,
	})
	if order.CancelReason != "market_halted" {
		t.Fatalf("order cancel reason=%q", order.CancelReason)
	}
	refused := decode.OrderFromProto(&orderv1.Order{
		OrderId: 2, Status: orderv1.OrderStatus_REJECTED,
		RejectReason: orderv1.ErrorCode_ERROR_CODE_MARKET_HALTED,
	})
	if refused.RejectReason != "MARKET_HALTED" {
		t.Fatalf("order reject reason=%q", refused.RejectReason)
	}

	events := decode.OrderUnitEventsFromProto([]*privatewsv1.UnitEvent{
		orderEvent(&privatewsv1.OrderEvent{OrderId: 3, Kind: privatewsv1.OrderEvent_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_MARKET_HALTED}),
		orderEvent(&privatewsv1.OrderEvent{OrderId: 4, Kind: privatewsv1.OrderEvent_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_MARKET_HALTED}),
	}, nil)
	if len(events) != 2 || events[0].Order.CancelReason != "market_halted" || events[1].Order.RejectReason != "MARKET_HALTED" {
		t.Fatalf("order stream events=%+v %+v", events[0].Order, events[1].Order)
	}

	trigger := decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 5, Status: triggersv1.TriggerStatus_STATUS_CANCELED,
		TerminalReason: &triggersv1.Trigger_CancelReason{
			CancelReason: triggersv1.TriggerCancelReason_TRIGGER_CANCEL_REASON_MARKET_HALTED,
		},
	})
	if trigger.CancelReason != "market_halted" || trigger.FailureReason != "" {
		t.Fatalf("trigger=%+v", trigger)
	}
	event := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		TriggerId: 5, EventType: triggersv1.TriggerEventType_EVENT_CANCELED,
		TerminalReason: &triggersv1.TriggerEvent_CancelReason{
			CancelReason: triggersv1.TriggerCancelReason_TRIGGER_CANCEL_REASON_MARKET_HALTED,
		},
	})
	if event.CancelReason != "market_halted" {
		t.Fatalf("trigger event=%+v", event)
	}
}

// A trigger the halted market refuses after admission reads failure_reason
// 212, a value with no name, until the server reports
// TRIGGER_FAILURE_REASON_MARKET_HALTED. The read still decodes.
func TestTriggerFailedByHaltWithUnnamedReasonStillDecodes(t *testing.T) {
	trigger := decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 6, Status: triggersv1.TriggerStatus_STATUS_FAILED,
		TerminalReason: &triggersv1.Trigger_FailureReason{FailureReason: triggersv1.TriggerFailureReason(212)},
	})
	if trigger.Status != "failed" || trigger.FailureReason != "unknown(212)" {
		t.Fatalf("trigger=%+v", trigger)
	}
}
