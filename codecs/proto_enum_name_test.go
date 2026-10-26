package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

func TestProtoEnumNameLabelsValuesNewerThanTheSDK(t *testing.T) {
	unknown := triggersv1.TriggerFailureReason(9999)
	if got := codecs.ProtoEnumName(unknown, unknown.Number()); got != "unknown(9999)" {
		t.Fatalf("unknown enum label = %q", got)
	}
	known := triggersv1.TriggerFailureReason(triggersv1.TriggerFailureReason_value["TRIGGER_FAILURE_REASON_CHILD_ORDER_INCONSISTENT"])
	if got := codecs.ProtoEnumName(known, known.Number()); got == "" || got == "unknown(235)" {
		t.Fatalf("known enum label = %q", got)
	}
}
