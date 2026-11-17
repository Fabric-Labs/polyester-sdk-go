package polyester

import (
	"context"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Triggers.Modify refuses a trailing distance the contract's
// ModifyTriggerRequest does not take, as create does: ticks must be above 0
// and bps within 1 to 10000. Nothing is sent. A positive distance is sent
// unchanged, and leaving it unset sends no trailing distance ("no change").
func TestTriggersModifyRefusesATrailingDistanceOutsideTheContract(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	modify := func(opts codecs.ModifyTriggerOptions) error {
		_, err := client.Triggers.Modify(ctx, nil, "2", nil, nil, nil, opts)
		return err
	}
	ticks := func(v int64) *int64 { return &v }
	bps := func(v int32) *int32 { return &v }
	for name, opts := range map[string]codecs.ModifyTriggerOptions{
		"ticks 0":   {TrailingDistanceTicks: ticks(0)},
		"ticks -5":  {TrailingDistanceTicks: ticks(-5)},
		"bps 0":     {TrailingDistanceBps: bps(0)},
		"bps 10001": {TrailingDistanceBps: bps(10001)},
	} {
		if err := modify(opts); !isValidationNaming(err, "trailing_distance") {
			t.Errorf("%s: err = %T %v, want a ValidationError naming the trailing distance", name, err, err)
		}
		if sent := capture.take(); len(sent) != 0 {
			t.Errorf("%s: sent %+v", name, sent)
		}
	}
	sentModify := func(name string) *triggersv1.ModifyTriggerRequest {
		t.Helper()
		sent := capture.take()
		if len(sent) == 0 {
			t.Fatalf("%s: nothing sent", name)
		}
		return sent[0].msg.(*triggersv1.ModifyTriggerRequest)
	}
	_ = modify(codecs.ModifyTriggerOptions{TrailingDistanceTicks: ticks(1_000_000_000)})
	if got := sentModify("ticks 1e9").GetTrailingDistanceTicks(); got != 1_000_000_000 {
		t.Errorf("ticks 1e9: sent %d", got)
	}
	for _, v := range []int32{1, 10000} {
		_ = modify(codecs.ModifyTriggerOptions{TrailingDistanceBps: bps(v)})
		if got := sentModify("bps").GetTrailingDistanceBps(); got != v {
			t.Errorf("bps %d: sent %d", v, got)
		}
	}
	price := models.PriceFromDecimal("100")
	_ = modify(codecs.ModifyTriggerOptions{TriggerPrice: &price})
	if req := sentModify("unset"); req.GetTrailingDistance() != nil {
		t.Errorf("unset trailing distance: sent %v", req.GetTrailingDistance())
	}
}
