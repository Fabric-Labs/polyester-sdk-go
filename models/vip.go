package models

import "time"

// VIPTier is one VIP0–VIP10 catalog row.
type VIPTier struct {
	Tier                uint32  `json:"tier,omitempty"`
	VolumeThresholdUsd  string  `json:"volume_threshold_usd,omitempty"`
	AopThresholdUsd     *string `json:"aop_threshold_usd,omitempty"`
	MakerFeeRatePercent string  `json:"maker_fee_rate_percent,omitempty"`
	TakerFeeRatePercent string  `json:"taker_fee_rate_percent,omitempty"`
}

// VIPTiersList is the complete active VIP policy catalog.
type VIPTiersList struct {
	PolicyVersion        uint64     `json:"policy_version,omitempty"`
	EffectiveFrom        *time.Time `json:"effective_from,omitempty"`
	RetentionThresholdBp uint32     `json:"retention_threshold_bp,omitempty"`
	Tiers                []VIPTier  `json:"tiers"`
}

// NextVIPTierThresholds holds entry thresholds for the tier above the effective tier.
type NextVIPTierThresholds struct {
	Tier               uint32 `json:"tier,omitempty"`
	VolumeThresholdUsd string `json:"volume_threshold_usd,omitempty"`
	AopThresholdUsd    string `json:"aop_threshold_usd,omitempty"`
}

// VIPStatus is authenticated caller-root VIP assignment and qualification facts.
type VIPStatus struct {
	Tier                uint32                 `json:"tier,omitempty"`
	VolumeTier          uint32                 `json:"volume_tier,omitempty"`
	AopTier             uint32                 `json:"aop_tier,omitempty"`
	SettledVolume30DUsd *string                `json:"settled_volume_30d_usd,omitempty"`
	AverageAop30DUsd    *string                `json:"average_aop_30d_usd,omitempty"`
	PolicyVersion       uint64                 `json:"policy_version,omitempty"`
	PolicyEffectiveFrom *time.Time             `json:"policy_effective_from,omitempty"`
	EffectiveFrom       *time.Time             `json:"effective_from,omitempty"`
	EvaluatedAt         *time.Time             `json:"evaluated_at,omitempty"`
	MetricsAsOf         *time.Time             `json:"metrics_as_of,omitempty"`
	NextTierThresholds  *NextVIPTierThresholds `json:"next_tier_thresholds,omitempty"`
}
