package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

func TestTriggerFromProtoMapsStopPrice(t *testing.T) {
	msg := &triggersv1.Trigger{
		TriggerId: 7,
		SymbolId:  2,
		Status:    triggersv1.TriggerStatus_STATUS_ARMED,
		QtyScaled: 1_000_000,
		Configuration: &triggersv1.Trigger_StopLoss{
			StopLoss: &triggersv1.ConditionalTrigger{
				TriggerPriceTicks: 50_000_000_000,
				Side:              orderv1.Side_SELL,
				Child: &triggersv1.ConditionalChildExecution{
					Execution: &triggersv1.ConditionalChildExecution_MarketIoc{MarketIoc: &triggersv1.TriggerMarketIoc{}},
				},
			},
		},
		RuntimeDetails: &triggersv1.Trigger_Stop{
			Stop: &triggersv1.StopDetails{TriggerPriceTicks: 50_000_000_000},
		},
		ClientTriggerId: "ct-1",
	}
	trigger := decode.TriggerFromProto(msg)
	if trigger.TriggerID != codecs.FormatUint64ID(7) {
		t.Fatalf("trigger_id=%q", trigger.TriggerID)
	}
	if trigger.TriggerPrice.Ticks() != 50_000_000_000 || trigger.ClientTriggerID != "ct-1" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if trigger.TriggerType != "stop_loss" || trigger.Status != "armed" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if trigger.Side != "sell" || trigger.OrderType != "market" {
		t.Fatalf("trigger side/type=%+v", trigger)
	}
	if trigger.Details == nil || trigger.Details.Case != "stop" {
		t.Fatalf("details=%+v", trigger.Details)
	}
	if trigger.SymbolID != 2 || trigger.Symbol != "" {
		t.Fatalf("symbol decode=%+v", trigger)
	}
}

func TestTriggerFromProtoProjectsTrailingStopSideAndParent(t *testing.T) {
	parentID := uint64(99)
	msg := &triggersv1.Trigger{
		TriggerId:     21,
		SymbolId:      1,
		Status:        triggersv1.TriggerStatus_STATUS_ARMED,
		QtyScaled:     50_000_000,
		ParentOrderId: &parentID,
		Configuration: &triggersv1.Trigger_TrailingStop{
			TrailingStop: &triggersv1.TrailingStopTrigger{
				TrailingDistance: &triggersv1.TrailingStopTrigger_TrailingDistanceBps{
					TrailingDistanceBps: 25,
				},
				Side: orderv1.Side_BUY,
			},
		},
	}
	trigger := decode.TriggerFromProto(msg)
	if trigger.TriggerType != "trailing_stop" {
		t.Fatalf("type=%q", trigger.TriggerType)
	}
	if trigger.Side != "buy" {
		t.Fatalf("side=%q (want buy from wire, not unspecified)", trigger.Side)
	}
	if trigger.OrderType != "market" || trigger.TimeInForce != "ioc" {
		t.Fatalf("child projection=%+v", trigger)
	}
	if trigger.ParentOrderID != codecs.FormatUint64ID(99) {
		t.Fatalf("parent_order_id=%q", trigger.ParentOrderID)
	}
}

func TestTriggerFromProtoProjectsTwapExecutedQty(t *testing.T) {
	msg := &triggersv1.Trigger{
		TriggerId: 11,
		SymbolId:  1,
		Status:    triggersv1.TriggerStatus_STATUS_RUNNING,
		QtyScaled: 100_000_000,
		Configuration: &triggersv1.Trigger_Twap{
			Twap: &triggersv1.TwapTrigger{
				Side:            orderv1.Side_BUY,
				DurationMs:      60_000,
				SliceIntervalMs: 5_000,
				Execution: &triggersv1.TwapTrigger_MarketIoc{
					MarketIoc: &triggersv1.TwapMarketIoc{},
				},
			},
		},
		RuntimeDetails: &triggersv1.Trigger_TwapState{
			TwapState: &triggersv1.TwapDetails{
				TwapDurationMs:      60_000,
				TwapSliceIntervalMs: 5_000,
				SliceIdx:            2,
				SliceCount:          12,
				ExecutedQtyScaled:   25_000_000,
			},
		},
		ClientTriggerId: "twap-1",
	}
	trigger := decode.TriggerFromProto(msg)
	if trigger.TriggerType != "twap" || trigger.Side != "buy" || trigger.OrderType != "market" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if trigger.Details == nil || trigger.Details.Case != "twap" {
		t.Fatalf("details=%+v", trigger.Details)
	}
	if trigger.Details.SliceIdx != 2 || trigger.Details.SliceCount != 12 {
		t.Fatalf("twap slices=%+v", trigger.Details)
	}
	if trigger.Details.ExecutedQty.Scaled() != 25_000_000 {
		t.Fatalf("executed_qty=%+v", trigger.Details.ExecutedQty)
	}
}

func TestTriggerStatusFromLabel(t *testing.T) {
	st, err := decode.TriggerStatusFromLabel("armed")
	if err != nil || st != triggersv1.TriggerStatus_STATUS_ARMED {
		t.Fatalf("armed: %v %v", st, err)
	}
	if _, err := decode.TriggerStatusFromLabel("nope"); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestTriggersListFromProto(t *testing.T) {
	msg := &triggersv1.ListTriggersResponse{
		Triggers:      []*triggersv1.Trigger{{TriggerId: 1, SymbolId: 1}},
		NextPageToken: "trig-page-2",
	}
	result := decode.TriggersListFromProto(msg)
	if len(result.Triggers) != 1 || result.Total != 1 || result.NextPageToken != "trig-page-2" {
		t.Fatalf("result=%+v", result)
	}
}

func TestTriggerEventsListFromProto(t *testing.T) {
	msg := &triggersv1.ListTriggerEventsResponse{
		Events: []*triggersv1.TriggerEvent{{
			TriggerId:      1,
			SubaccountId:   9,
			SymbolId:       2,
			TriggerType:    triggersv1.TriggerType_TAKE_PROFIT,
			EventType:      triggersv1.TriggerEventType_EVENT_CANCELED,
			TsNs:           123,
			ChildSeq:       3,
			ChildOrderId:   77,
			FirePriceTicks: int64Ptr(100),
			TerminalReason: &triggersv1.TriggerEvent_CancelReason{
				CancelReason: triggersv1.TriggerCancelReason_TRIGGER_CANCEL_REASON_USER_REQUEST,
			},
		}},
		NextPageToken: "evt-page-2",
	}
	result := decode.TriggerEventsListFromProto(msg)
	if len(result.Events) != 1 || result.NextPageToken != "evt-page-2" {
		t.Fatalf("result=%+v", result)
	}
	ev := result.Events[0]
	if ev.EventType != "canceled" || ev.TriggerType != "take_profit" {
		t.Fatalf("labels=%+v", ev)
	}
	if ev.SubaccountID != codecs.FormatUint64ID(9) || ev.ChildOrderID != codecs.FormatUint64ID(77) {
		t.Fatalf("ids=%+v", ev)
	}
	if ev.ChildSeq != 3 || ev.FirePrice.Ticks() != 100 || ev.CancelReason != "user_request" || ev.FailureReason != "" {
		t.Fatalf("detail=%+v", ev)
	}
}

func TestTriggerEventTypeFromLabel(t *testing.T) {
	et, err := decode.TriggerEventTypeFromLabel("fired")
	if err != nil || et != triggersv1.TriggerEventType_EVENT_FIRED {
		t.Fatalf("fired: %v %v", et, err)
	}
	if _, err := decode.TriggerEventTypeFromLabel("nope"); err == nil {
		t.Fatal("expected error for invalid event type")
	}
}

func TestGetTriggerFromProto(t *testing.T) {
	msg := &triggersv1.GetTriggerResponse{
		Trigger: &triggersv1.Trigger{TriggerId: 3, SymbolId: 1},
	}
	trigger := decode.GetTriggerFromProto(msg)
	if trigger == nil || trigger.TriggerID != codecs.FormatUint64ID(3) {
		t.Fatalf("trigger=%+v", trigger)
	}
}

func TestTriggerEventFailureReasonAndUnspecified(t *testing.T) {
	failed := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		TriggerId: 2,
		EventType: triggersv1.TriggerEventType_EVENT_FAILED,
		TerminalReason: &triggersv1.TriggerEvent_FailureReason{
			FailureReason: triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_INSUFFICIENT_FUNDS,
		},
	})
	if failed.EventType != "failed" || failed.FailureReason != "insufficient_funds" || failed.CancelReason != "" {
		t.Fatalf("failed=%+v", failed)
	}
	unspecified := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		TriggerId: 3,
		EventType: triggersv1.TriggerEventType_EVENT_CANCELED,
		TerminalReason: &triggersv1.TriggerEvent_CancelReason{
			CancelReason: triggersv1.TriggerCancelReason_TRIGGER_CANCEL_REASON_UNSPECIFIED,
		},
	})
	if unspecified.CancelReason != "" || unspecified.FailureReason != "" {
		t.Fatalf("unspecified=%+v", unspecified)
	}
	armed := decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 4,
		Status:    triggersv1.TriggerStatus_STATUS_ARMED,
		TerminalReason: &triggersv1.Trigger_FailureReason{
			FailureReason: triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_MARKET_HALTED,
		},
	})
	if armed.FailureReason != "market_halted" || armed.CancelReason != "" {
		t.Fatalf("trigger=%+v", armed)
	}
}

func TestTriggerEventAbsentFirePrice(t *testing.T) {
	msg := &triggersv1.ListTriggerEventsResponse{
		Events: []*triggersv1.TriggerEvent{{
			TriggerId:   1,
			EventType:   triggersv1.TriggerEventType_EVENT_FIRED,
			TriggerType: triggersv1.TriggerType_TWAP,
			ChildSeq:    1,
		}},
	}
	result := decode.TriggerEventsListFromProto(msg)
	if len(result.Events) != 1 {
		t.Fatalf("events=%+v", result.Events)
	}
	if result.Events[0].FirePrice.Ticks() != 0 {
		t.Fatalf("expected absent fire price, got %+v", result.Events[0].FirePrice)
	}
}

func int64Ptr(v int64) *int64 { return &v }
