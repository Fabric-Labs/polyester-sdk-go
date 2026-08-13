package models

// SpotFeeRate is the effective maker/taker rates for one spot market and account target.
type SpotFeeRate struct {
	SymbolID            uint32 `json:"symbol_id,omitempty"`
	Symbol              string `json:"symbol,omitempty"`
	MakerFeeRatePercent string `json:"maker_fee_rate_percent,omitempty"`
	TakerFeeRatePercent string `json:"taker_fee_rate_percent,omitempty"`
	VIPTier             uint32 `json:"vip_tier,omitempty"`
}

// SpotFeeRatesList is effective spot fee rows ordered by numeric market identifier.
type SpotFeeRatesList struct {
	FeeRates []SpotFeeRate `json:"fee_rates"`
}
