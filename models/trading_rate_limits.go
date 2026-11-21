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
	// Rules are the placement, cancellation and read rules enforced for the
	// account, in that order; select one by PolicyClass, not by position.
	// The read rule (TRADING_RATE_LIMIT_CLASS_READ) is the selected
	// account's own: the root account and each sub-account read on separate
	// budgets, so reading one sub-account never uses another's. Order,
	// trade, trigger, trading-limit and fee reads, and order previews, draw
	// on the read limit of the account they read, shared by every API key
	// and session reading it. A sub-account reads at its own read-limit
	// override, else at its root account's VIP tier read limit, never on the
	// root account's override. Budget against these, per account.
	Rules []TradingRateLimitRule `json:"rules"`
	// APIKeyRules are the rules resolved for the calling API key, present only
	// when the caller used one. Its read rule is enforced on top of the
	// account's: a read is refused when either is exhausted, so pace reads per
	// API key too when the key has its own read rule. Its placement and
	// cancellation rules are informational: trading requests are not limited
	// per API key.
	APIKeyRules []TradingRateLimitRule `json:"api_key_rules"`
}
