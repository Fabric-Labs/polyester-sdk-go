package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	analyticsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
)

func TestResolveAnalyticsRange(t *testing.T) {
	rng, err := codecs.ResolveAnalyticsRange("7d")
	if err != nil {
		t.Fatal(err)
	}
	if rng != analyticsv1.ChainAnalyticsRange_DAY_7 {
		t.Fatalf("range=%v", rng)
	}
}

func TestResolveAnalyticsRangeInvalid(t *testing.T) {
	_, err := codecs.ResolveAnalyticsRange("2w")
	if err == nil {
		t.Fatal("expected validation error")
	}
}
