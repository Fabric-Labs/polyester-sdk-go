package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"google.golang.org/protobuf/proto"
)

// A refused fire is a non-terminal event: it names the attempt's child_seq,
// its fire price and why the order was refused, places no order and sets no
// terminal reason.
func TestFireRefusedEventCarriesItsReason(t *testing.T) {
	reason := triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_RATE_LIMITED
	got := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		TriggerId:         11,
		EventType:         triggersv1.TriggerEventType_EVENT_FIRE_REFUSED,
		ChildSeq:          1,
		FirePriceTicks:    proto.Int64(104_000_000_000),
		TsNs:              5,
		FireRefusalReason: &reason,
	})
	if got.EventType != "fire_refused" || got.FireRefusalReason != "rate_limited" || got.ChildSeq != 1 ||
		got.FirePrice.Ticks() != 104_000_000_000 || got.ChildOrderID != "" || got.CancelReason != "" || got.FailureReason != "" {
		t.Fatalf("event=%+v", got)
	}
	repairing := triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_ACCOUNT_REPAIRING
	got = decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		EventType: triggersv1.TriggerEventType_EVENT_FIRE_REFUSED, FireRefusalReason: &repairing,
	})
	if got.FireRefusalReason != "account_repairing" {
		t.Fatalf("reason=%q", got.FireRefusalReason)
	}
	failed := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		EventType:      triggersv1.TriggerEventType_EVENT_FAILED,
		TerminalReason: &triggersv1.TriggerEvent_FailureReason{FailureReason: repairing},
	})
	if failed.FailureReason != "account_repairing" || failed.FireRefusalReason != "" {
		t.Fatalf("failed=%+v", failed)
	}
}

func TestTriggerEventsTolerateUnknownValues(t *testing.T) {
	unknownReason := triggersv1.TriggerFailureReason(777)
	got := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		EventType: triggersv1.TriggerEventType(99), FireRefusalReason: &unknownReason,
	})
	if got.EventType != "unknown(99)" || got.FireRefusalReason != "unknown(777)" {
		t.Fatalf("event=%+v", got)
	}
}

func TestFireRefusedFiltersAndStreams(t *testing.T) {
	eventType, err := decode.TriggerEventTypeFromLabel("fire_refused")
	if err != nil || eventType != triggersv1.TriggerEventType_EVENT_FIRE_REFUSED {
		t.Fatalf("eventType=%v err=%v", eventType, err)
	}
	reason := triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_MIN_NOTIONAL
	payload, err := proto.Marshal(&triggersv1.TriggerEventBatch{Events: []*triggersv1.TriggerEvent{{
		TriggerId: 3, EventType: triggersv1.TriggerEventType_EVENT_FIRE_REFUSED, ChildSeq: 2, FireRefusalReason: &reason,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := decode.TriggerEventBatchFromBytes(payload)
	if err != nil || len(events) != 1 || events[0].EventType != "fire_refused" || events[0].FireRefusalReason != "min_notional" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}
