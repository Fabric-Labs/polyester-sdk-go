package decode_test

import (
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ratelimit/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRateLimitConfigUsesFullPolicyClassNames(t *testing.T) {
	msg := &ratelimitv1.GetRateLimitConfigResponse{
		PolicyVersion: 9,
		EffectiveFrom: &timestamppb.Timestamp{Seconds: 50},
		Rules: []*ratelimitv1.TradingRateLimitRule{
			{
				PolicyClass: ratelimitv1.TradingRateLimitClass_TRADING_RATE_LIMIT_CLASS_PLACE,
				Tier:        0,
				QuotaWeight: 100,
				PeriodMs:    1000,
				BurstWeight: 20,
			},
			{
				PolicyClass: ratelimitv1.TradingRateLimitClass(99),
				Tier:        1,
				QuotaWeight: 50,
				PeriodMs:    1000,
				BurstWeight: 10,
			},
		},
	}
	result := decode.RateLimitConfigFromProto(msg)
	if result.PolicyVersion != 9 {
		t.Fatalf("policy_version=%d", result.PolicyVersion)
	}
	wantFrom := time.Unix(50, 0).UTC()
	if result.EffectiveFrom == nil || !result.EffectiveFrom.Equal(wantFrom) {
		t.Fatalf("effective_from=%v", result.EffectiveFrom)
	}
	if len(result.Rules) != 2 {
		t.Fatalf("rules=%d", len(result.Rules))
	}
	if result.Rules[0].PolicyClass != "TRADING_RATE_LIMIT_CLASS_PLACE" {
		t.Fatalf("place class=%q", result.Rules[0].PolicyClass)
	}
	if result.Rules[1].PolicyClass != "UNKNOWN_TRADING_RATE_LIMIT_CLASS(99)" {
		t.Fatalf("unknown class=%q", result.Rules[1].PolicyClass)
	}
}

func TestTradingRateLimitsDecodeAccountAndAPIKeyRules(t *testing.T) {
	rule := &ratelimitv1.TradingRateLimitRule{
		PolicyClass: ratelimitv1.TradingRateLimitClass_TRADING_RATE_LIMIT_CLASS_CANCEL,
		Tier:        3,
		QuotaWeight: 200,
		PeriodMs:    500,
		BurstWeight: 40,
	}
	msg := &ratelimitv1.GetTradingRateLimitsResponse{
		PolicyVersion: 4,
		Rules:         []*ratelimitv1.TradingRateLimitRule{rule},
		ApiKeyRules:   []*ratelimitv1.TradingRateLimitRule{rule},
	}
	result := decode.TradingRateLimitsFromProto(msg)
	if result.EffectiveFrom != nil {
		t.Fatalf("effective_from=%v", result.EffectiveFrom)
	}
	if len(result.Rules) != 1 || result.Rules[0].PolicyClass != "TRADING_RATE_LIMIT_CLASS_CANCEL" {
		t.Fatalf("rules=%+v", result.Rules)
	}
	if len(result.APIKeyRules) != 1 || result.APIKeyRules[0].Tier != 3 {
		t.Fatalf("api_key_rules=%+v", result.APIKeyRules)
	}
}
