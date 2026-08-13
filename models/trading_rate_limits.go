package models

import "time"

// TradingRateLimitRule is a weighted placement or cancellation quota for one VIP tier.
type TradingRateLimitRule struct {
	PolicyClass string `json:"policy_class,omitempty"`
	Tier        uint32 `json:"tier,omitempty"`
	QuotaWeight uint64 `json:"quota_weight,omitempty"`
	PeriodMs    uint64 `json:"period_ms,omitempty"`
	BurstWeight uint64 `json:"burst_weight,omitempty"`
}

// RateLimitConfig is the complete public trading rate-limit catalog for one policy version.
type RateLimitConfig struct {
	PolicyVersion uint64                 `json:"policy_version,omitempty"`
	EffectiveFrom *time.Time             `json:"effective_from,omitempty"`
	Rules         []TradingRateLimitRule `json:"rules"`
}

// TradingRateLimits is the effective trading limits for one account target and caller.
type TradingRateLimits struct {
	PolicyVersion uint64                 `json:"policy_version,omitempty"`
	EffectiveFrom *time.Time             `json:"effective_from,omitempty"`
	Rules         []TradingRateLimitRule `json:"rules"`
	APIKeyRules   []TradingRateLimitRule `json:"api_key_rules"`
}
