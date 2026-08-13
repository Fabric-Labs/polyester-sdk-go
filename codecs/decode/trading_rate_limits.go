package decode

import (
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ratelimit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// TradingRateLimitRuleFromProto decodes one weighted quota rule.
// PolicyClass uses the full protobuf enum name; unknown values become
// UNKNOWN_TRADING_RATE_LIMIT_CLASS(n).
func TradingRateLimitRuleFromProto(msg *ratelimitv1.TradingRateLimitRule) models.TradingRateLimitRule {
	if msg == nil {
		return models.TradingRateLimitRule{}
	}
	return models.TradingRateLimitRule{
		PolicyClass: enumNameOrUnknown(
			ratelimitv1.TradingRateLimitClass_name,
			int32(msg.GetPolicyClass()),
			"UNKNOWN_TRADING_RATE_LIMIT_CLASS",
		),
		Tier:        msg.GetTier(),
		QuotaWeight: msg.GetQuotaWeight(),
		PeriodMs:    msg.GetPeriodMs(),
		BurstWeight: msg.GetBurstWeight(),
	}
}

func tradingRateLimitRulesFromProto(items []*ratelimitv1.TradingRateLimitRule) []models.TradingRateLimitRule {
	out := make([]models.TradingRateLimitRule, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, TradingRateLimitRuleFromProto(item))
	}
	return out
}

// RateLimitConfigFromProto decodes GetRateLimitConfigResponse.
func RateLimitConfigFromProto(msg *ratelimitv1.GetRateLimitConfigResponse) models.RateLimitConfig {
	return models.RateLimitConfig{
		PolicyVersion: msg.GetPolicyVersion(),
		EffectiveFrom: timestampTime(msg.GetEffectiveFrom()),
		Rules:         tradingRateLimitRulesFromProto(msg.GetRules()),
	}
}

// TradingRateLimitsFromProto decodes GetTradingRateLimitsResponse.
func TradingRateLimitsFromProto(msg *ratelimitv1.GetTradingRateLimitsResponse) models.TradingRateLimits {
	return models.TradingRateLimits{
		PolicyVersion: msg.GetPolicyVersion(),
		EffectiveFrom: timestampTime(msg.GetEffectiveFrom()),
		Rules:         tradingRateLimitRulesFromProto(msg.GetRules()),
		APIKeyRules:   tradingRateLimitRulesFromProto(msg.GetApiKeyRules()),
	}
}
