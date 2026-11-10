package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// POLY-5724: a trailing distance in ticks is a price delta in 1e-9 quote
// units, the scale of every price tick, and every path sends it unchanged:
// 50 USDT is 50_000_000_000 ticks and 0.5 USDT is 500_000_000.
func TestTrailingDistanceTicksArePriceTicks(t *testing.T) {
	for _, tc := range []struct {
		distance string
		ticks    int64
	}{
		{"50", 50_000_000_000},
		{"0.5", 500_000_000},
		{"0.000000001", 1},
	} {
		// The README's conversion.
		parsed, err := codecs.ParsePriceTicks(tc.distance, "trailing distance")
		if err != nil || int64(parsed) != tc.ticks {
			t.Fatalf("ParsePriceTicks(%q) = %d, %v; want %d", tc.distance, parsed, err, tc.ticks)
		}
		distance := int64(parsed)

		create, err := codecs.CreateTriggerToProto(1, "BTC-USDT", "trailing_stop", nil, "sell",
			models.QtyFromDecimal("0.1"), "market", nil, "", "", nil, nil, false, 8,
			codecs.CreateTriggerOptions{TrailingDistanceTicks: &distance})
		if err != nil {
			t.Fatal(err)
		}
		if got := create.GetTrigger().GetTrailingStop().GetTrailingDistanceTicks(); got != tc.ticks {
			t.Fatalf("create %s: trailing_distance_ticks=%d want %d", tc.distance, got, tc.ticks)
		}

		modify, err := codecs.ModifyTriggerToProto("7", 0, "", nil, codecs.ModifyTriggerOptions{TrailingDistanceTicks: &distance})
		if err != nil {
			t.Fatal(err)
		}
		if got := modify.GetTrailingDistanceTicks(); got != tc.ticks {
			t.Fatalf("modify %s: trailing_distance_ticks=%d want %d", tc.distance, got, tc.ticks)
		}

		policy, err := codecs.RiskPolicyToProto(&models.AttachedRiskInput{
			TrailingStop: &models.TrailingStopInput{DistanceTicks: &distance},
		}, "order.attached_risk", "BTC-USDT")
		if err != nil {
			t.Fatal(err)
		}
		if got := policy.GetTrailingStop().GetTrailingDistanceTicks(); got != tc.ticks {
			t.Fatalf("attached %s: trailing_distance_ticks=%d want %d", tc.distance, got, tc.ticks)
		}

		read := decode.TriggerFromProto(&triggersv1.Trigger{
			RuntimeDetails: &triggersv1.Trigger_Trailing{Trailing: &triggersv1.TrailingDetails{TrailingDistanceTicks: tc.ticks}},
		})
		if read.Details == nil || read.Details.TrailingDistance.Ticks() != tc.ticks {
			t.Fatalf("read %s: details=%+v", tc.distance, read.Details)
		}
	}
}
