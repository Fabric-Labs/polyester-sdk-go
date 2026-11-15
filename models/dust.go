package models

import "time"

// Dust conversion turns dust, Trading balances too small to trade, into one
// target asset at index prices less a disclosed fee. Every amount below is
// exact: 18-decimal values decoded from their U128 without rounding, never
// floats.

// DustBalance is one Trading balance judged for dust conversion.
type DustBalance struct {
	AssetID uint32 `json:"asset_id"`
	// Amount is the whole available Trading balance, at the 18-decimal
	// ledger scale: what no open order or other reservation holds.
	Amount AssetAmountScaled `json:"amount"`
	// Eligibility is "eligible" when the whole balance converts; otherwise
	// why not: "target_asset", "tradable" (an open market accepts an order
	// for it: sell or spend it instead), "above_value_limit", "no_price",
	// "unsupported_asset" or "source_limit" (one conversion takes at most 20
	// balances; this one waits for the next). "unknown(n)" for a value newer
	// than this SDK.
	Eligibility string `json:"eligibility"`
	// USDValue is the balance's USD value at the 18-decimal scale, rounded
	// down; nil when the asset has no current price.
	USDValue *AssetAmountScaled `json:"usd_value,omitempty"`
}

// DustBalances is the answer to DustConversions.ListBalances: a preview that
// reserves nothing.
type DustBalances struct {
	// TargetAssetID is the target the balances were judged for.
	TargetAssetID uint32 `json:"target_asset_id"`
	// Targets lists every asset the account can convert dust into, in
	// preference order.
	Targets []uint32 `json:"targets"`
	// FeeBps is the fee in basis points, taken from the payout.
	FeeBps uint32 `json:"fee_bps"`
	// Balances holds every nonzero available Trading balance, in asset ID
	// order.
	Balances []DustBalance `json:"balances"`
	// GrossUSD is the eligible balances' total USD value at the 18-decimal
	// scale, rounded down.
	GrossUSD AssetAmountScaled `json:"gross_usd"`
	// Payout is what they would bring now, in target units at the 18-decimal
	// scale, after the fee.
	Payout AssetAmountScaled `json:"payout"`
	// TargetAvailable is false when the target cannot be paid now; a quote
	// is then refused with ERROR_CODE_DUST_TARGET_UNAVAILABLE.
	TargetAvailable bool `json:"target_available"`
}

// DustAssetPrice is the USD index price a quote used for one asset.
type DustAssetPrice struct {
	AssetID uint32 `json:"asset_id"`
	// Price is USD per whole unit of the asset, exactly PriceTicks /
	// 10^PriceScale, as a decimal string.
	Price      string `json:"price"`
	PriceTicks int64  `json:"price_ticks"`
	PriceScale uint32 `json:"price_scale"`
	// AsOf is the time of the oldest index observation the price used.
	AsOf *time.Time `json:"as_of,omitempty"`
}

// DustQuote is the answer to DustConversions.Quote, valid until ExpiresAt
// (30 seconds). It reserves nothing.
type DustQuote struct {
	// QuoteID is what DustConversions.Convert takes; empty when no balance is
	// eligible or the payout rounds to zero (Balances still says why).
	QuoteID       string     `json:"quote_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	TargetAssetID uint32     `json:"target_asset_id"`
	FeeBps        uint32     `json:"fee_bps"`
	// GrossUSD is the sources' total USD value at the 18-decimal scale,
	// rounded down.
	GrossUSD AssetAmountScaled `json:"gross_usd"`
	// Payout is in target units at the 18-decimal scale, after the fee,
	// rounded down to the target's quantity precision.
	Payout AssetAmountScaled `json:"payout"`
	// Balances holds every nonzero available Trading balance, sources and
	// others, in asset ID order.
	Balances []DustBalance `json:"balances"`
	// Prices holds the prices of the sources and the target, in asset ID
	// order.
	Prices []DustAssetPrice `json:"prices"`
}

// DustSource is one balance a conversion moves.
type DustSource struct {
	AssetID uint32 `json:"asset_id"`
	// Amount is the whole quoted balance at the 18-decimal ledger scale.
	Amount AssetAmountScaled `json:"amount"`
}

// DustConversion is one conversion of an account's dust into a target asset.
//
// State is "processing" (accepted; it completes or is refused without any
// further request: read it again with DustConversions.Get), "completed"
// (every source moved and the payout was credited, atomically) or "refused"
// (nothing moved; RefusalCode says why; it does not count toward the 6-hour
// conversion window). "unknown(n)" for a state newer than this SDK.
type DustConversion struct {
	ConversionID string `json:"conversion_id"`
	QuoteID      string `json:"quote_id,omitempty"`
	State        string `json:"state"`
	// RefusalCode is why a refused conversion was refused, as a lowercase
	// label without the ERROR_CODE_ prefix, such as "dust_price_moved"; empty
	// unless State is "refused", and UNKNOWN_ERROR_CODE(n) for a code newer
	// than this SDK.
	RefusalCode   string       `json:"refusal_code,omitempty"`
	TargetAssetID uint32       `json:"target_asset_id"`
	Sources       []DustSource `json:"sources"`
	// Payout, QuotedPayout and Fee are in target units at the 18-decimal
	// scale: the payout after the fee, the payout the quote showed, and the
	// fee taken from the payout.
	Payout       AssetAmountScaled `json:"payout"`
	QuotedPayout AssetAmountScaled `json:"quoted_payout"`
	Fee          AssetAmountScaled `json:"fee"`
	FeeBps       uint32            `json:"fee_bps"`
	CreatedAt    *time.Time        `json:"created_at,omitempty"`
	// CompletedAt is set once State is "completed".
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// DustConversionsList is one page of conversions, newest first.
type DustConversionsList struct {
	Conversions   []DustConversion `json:"conversions"`
	NextPageToken string           `json:"next_page_token,omitempty"`
}
