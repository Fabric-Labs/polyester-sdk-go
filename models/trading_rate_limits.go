package models

import "time"

// TradingRateLimitRule is a weighted placement or cancellation quota for one VIP tier.
type TradingRateLimitRule struct {
	PolicyClass string `json:"policy_class,omitempty"`
	VIPTier     uint32 `json:"vip_tier,omitempty"`
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

// TradingRateLimits is the trading limits enforced for one account target.
// Limits apply per account: the root account and each sub-account have their
// own, and every API key and session trading for an account shares them, so
// adding a key adds no capacity.
type TradingRateLimits struct {
	PolicyVersion uint64     `json:"policy_version,omitempty"`
	EffectiveFrom *time.Time `json:"effective_from,omitempty"`
	// Rules are the placement and cancellation rules enforced for the
	// account, placement first. Budget against these.
	Rules []TradingRateLimitRule `json:"rules"`
	// APIKeyRules are the rules resolved for the calling API key, present only
	// when the caller used one. They are informational: trading requests are
	// not limited per API key.
	APIKeyRules []TradingRateLimitRule `json:"api_key_rules"`
}
