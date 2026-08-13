package decode_test

import (
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	vipv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/vip/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestVIPTiersPreserveOptionalAopAndTimestamps(t *testing.T) {
	aop := "50000.5"
	msg := &vipv1.ListVIPTiersResponse{
		PolicyVersion:        7,
		EffectiveFrom:        &timestamppb.Timestamp{Seconds: 1_700_000_000, Nanos: 250_000_000},
		RetentionThresholdBp: 9500,
		Tiers: []*vipv1.VIPTier{
			{
				Tier:                0,
				VolumeThresholdUsd:  "0",
				MakerFeeRatePercent: "0.02",
				TakerFeeRatePercent: "0.05",
			},
			{
				Tier:                1,
				VolumeThresholdUsd:  "100000",
				AopThresholdUsd:     &aop,
				MakerFeeRatePercent: "0.01",
				TakerFeeRatePercent: "0.04",
			},
		},
	}
	result := decode.VIPTiersListFromProto(msg)
	if result.PolicyVersion != 7 || result.RetentionThresholdBp != 9500 {
		t.Fatalf("catalog=%+v", result)
	}
	wantFrom := time.Unix(1_700_000_000, 250_000_000).UTC()
	if result.EffectiveFrom == nil || !result.EffectiveFrom.Equal(wantFrom) {
		t.Fatalf("effective_from=%v want %v", result.EffectiveFrom, wantFrom)
	}
	if len(result.Tiers) != 2 {
		t.Fatalf("tiers=%d", len(result.Tiers))
	}
	if result.Tiers[0].AopThresholdUsd != nil {
		t.Fatalf("VIP0 aop should be nil, got %v", result.Tiers[0].AopThresholdUsd)
	}
	if result.Tiers[1].AopThresholdUsd == nil || *result.Tiers[1].AopThresholdUsd != "50000.5" {
		t.Fatalf("VIP1 aop=%v", result.Tiers[1].AopThresholdUsd)
	}
	if result.Tiers[1].VolumeThresholdUsd != "100000" {
		t.Fatalf("volume=%q", result.Tiers[1].VolumeThresholdUsd)
	}
}

func TestVIPStatusOmitsUnsetQualificationFields(t *testing.T) {
	msg := &vipv1.GetVIPStatusResponse{
		Tier:                0,
		VolumeTier:          0,
		AopTier:             0,
		PolicyVersion:       1,
		PolicyEffectiveFrom: &timestamppb.Timestamp{Seconds: 1_700_000_100},
	}
	status := decode.VIPStatusFromProto(msg)
	if status.Tier != 0 {
		t.Fatalf("tier=%d", status.Tier)
	}
	if status.SettledVolume30DUsd != nil || status.AverageAop30DUsd != nil {
		t.Fatalf("metrics should be nil: %+v", status)
	}
	if status.EffectiveFrom != nil || status.EvaluatedAt != nil || status.MetricsAsOf != nil {
		t.Fatalf("optional timestamps should be nil: %+v", status)
	}
	if status.NextTierThresholds != nil {
		t.Fatalf("next_tier_thresholds=%+v", status.NextTierThresholds)
	}
	wantPolicyFrom := time.Unix(1_700_000_100, 0).UTC()
	if status.PolicyEffectiveFrom == nil || !status.PolicyEffectiveFrom.Equal(wantPolicyFrom) {
		t.Fatalf("policy_effective_from=%v", status.PolicyEffectiveFrom)
	}
}

func TestVIPStatusSurfacesNextTierAndMetrics(t *testing.T) {
	settled := "250000.12"
	aop := "80000"
	msg := &vipv1.GetVIPStatusResponse{
		Tier:                 2,
		VolumeTier:           2,
		AopTier:              1,
		SettledVolume_30DUsd: &settled,
		AverageAop_30DUsd:    &aop,
		PolicyVersion:        3,
		PolicyEffectiveFrom:  &timestamppb.Timestamp{Seconds: 10},
		EffectiveFrom:        &timestamppb.Timestamp{Seconds: 20},
		EvaluatedAt:          &timestamppb.Timestamp{Seconds: 30},
		MetricsAsOf:          &timestamppb.Timestamp{Seconds: 40},
		NextTierThresholds: &vipv1.NextVIPTierThresholds{
			Tier:               3,
			VolumeThresholdUsd: "500000",
			AopThresholdUsd:    "150000",
		},
	}
	status := decode.VIPStatusFromProto(msg)
	if status.SettledVolume30DUsd == nil || *status.SettledVolume30DUsd != "250000.12" {
		t.Fatalf("settled=%v", status.SettledVolume30DUsd)
	}
	if status.NextTierThresholds == nil || status.NextTierThresholds.Tier != 3 {
		t.Fatalf("next=%+v", status.NextTierThresholds)
	}
	wantMetrics := time.Unix(40, 0).UTC()
	if status.MetricsAsOf == nil || !status.MetricsAsOf.Equal(wantMetrics) {
		t.Fatalf("metrics_as_of=%v", status.MetricsAsOf)
	}
}
