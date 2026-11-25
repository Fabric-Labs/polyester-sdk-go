package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// A TWAP or ladder that has executed nothing yet reads a zero executed
// quantity at the pair's scale, as its Qty does, so both format; a partly
// executed one formats its executed quantity too.
func TestTriggerExecutedQtyFormatsBeforeAndAfterExecution(t *testing.T) {
	scales := func(symbolID uint32) (int, bool) { return 8, symbolID == 1 }
	for _, tc := range []struct {
		name     string
		executed int64
		want     string
		build    func(executed int64) *triggersv1.Trigger
	}{
		{"twap unfilled", 0, "0", twapTrigger},
		{"twap partly executed", 25_000, "0.00025", twapTrigger},
		{"ladder unfilled", 0, "0", ladderTrigger},
		{"ladder partly executed", 25_000, "0.00025", ladderTrigger},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trigger := decode.ScaleTrigger(mustDecoded(decode.TriggerFromProto(tc.build(tc.executed))), scales)
			if got, err := trigger.Qty.Format(); err != nil || got != "0.0006" {
				t.Fatalf("Qty.Format() = %q, %v", got, err)
			}
			got, err := trigger.Details.ExecutedQty.Format()
			if err != nil || got != tc.want {
				t.Fatalf("Details.ExecutedQty.Format() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func twapTrigger(executed int64) *triggersv1.Trigger {
	return &triggersv1.Trigger{
		TriggerId: 1, SymbolId: 1, QtyScaled: 60_000, Status: triggersv1.TriggerStatus_STATUS_RUNNING,
		RuntimeDetails: &triggersv1.Trigger_TwapState{TwapState: &triggersv1.TwapDetails{
			TwapDurationMs: 20_000, TwapSliceIntervalMs: 10_000, SliceIdx: 1, SliceCount: 2, ExecutedQtyScaled: executed,
		}},
	}
}

func ladderTrigger(executed int64) *triggersv1.Trigger {
	return &triggersv1.Trigger{
		TriggerId: 2, SymbolId: 1, QtyScaled: 60_000, Status: triggersv1.TriggerStatus_STATUS_RUNNING,
		RuntimeDetails: &triggersv1.Trigger_LadderState{LadderState: &triggersv1.LadderDetails{
			LadderLevels: 3, ExecutedQtyScaled: executed,
		}},
	}
}
