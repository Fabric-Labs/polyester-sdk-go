package models

import "time"

// SpotFeeRate contains effective fill rates and the rates used to reserve quote fees.
type SpotFeeRate struct {
	SymbolID                      uint32                `json:"symbol_id,omitempty"`
	Symbol                        string                `json:"symbol,omitempty"`
	MakerFeeRatePercent           string                `json:"maker_fee_rate_percent,omitempty"`
	TakerFeeRatePercent           string                `json:"taker_fee_rate_percent,omitempty"`
	VIPTier                       uint32                `json:"vip_tier,omitempty"`
	ReserveFeeRatePercent         string                `json:"reserve_fee_rate_percent,omitempty"`
	PostOnlyReserveFeeRatePercent string                `json:"post_only_reserve_fee_rate_percent,omitempty"`
	NextChangeAt                  *time.Time            `json:"next_change_at,omitempty"`
	Breakdown                     *SpotFeeRateBreakdown `json:"breakdown,omitempty"`
}

// SpotFeeRateBreakdown explains the components of the effective fee rates.
type SpotFeeRateBreakdown struct {
	TierMakerFeeRatePercent string             `json:"tier_maker_fee_rate_percent,omitempty"`
	TierTakerFeeRatePercent string             `json:"tier_taker_fee_rate_percent,omitempty"`
	MarketMultiplier        *SpotFeeMultiplier `json:"market_multiplier,omitempty"`
	AccountDiscounts        []SpotFeeDiscount  `json:"account_discounts"`
	ReferralDiscount        *SpotFeeDiscount   `json:"referral_discount,omitempty"`
}

// SpotFeeMultiplier is a market fee schedule in force over a UTC interval.
type SpotFeeMultiplier struct {
	MakerMultiplierPercent string     `json:"maker_multiplier_percent,omitempty"`
	TakerMultiplierPercent string     `json:"taker_multiplier_percent,omitempty"`
	EffectiveFrom          *time.Time `json:"effective_from,omitempty"`
	EffectiveTo            *time.Time `json:"effective_to,omitempty"`
}

// SpotFeeDiscount is an account or referral discount in force over a UTC interval.
type SpotFeeDiscount struct {
	MakerDiscountPercent string     `json:"maker_discount_percent,omitempty"`
	TakerDiscountPercent string     `json:"taker_discount_percent,omitempty"`
	AllMarkets           bool       `json:"all_markets,omitempty"`
	EffectiveFrom        *time.Time `json:"effective_from,omitempty"`
	EffectiveTo          *time.Time `json:"effective_to,omitempty"`
}

// SpotFeeRatesList is effective spot fee rows ordered by numeric market identifier.
type SpotFeeRatesList struct {
	FeeRates []SpotFeeRate `json:"fee_rates"`
}
