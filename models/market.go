package models

import "time"

// Candle is one OHLCV candle.
type Candle struct {
	TsSec       int64  `json:"ts_sec,omitempty"`
	Open        string `json:"open,omitempty"`
	High        string `json:"high,omitempty"`
	Low         string `json:"low,omitempty"`
	Close       string `json:"close,omitempty"`
	Volume      string `json:"volume,omitempty"`
	QuoteVolume string `json:"quote_volume,omitempty"`
	// IsClosed is true for a closed candle bucket, including historical rows
	// and the terminal live update, and false only while the bucket is still
	// open. The columnar response has no closed flag: GetCandlesColumns rows
	// are true, except the newest primary row of a first page read with
	// incomplete candles, which reads false, meaning not known to be closed.
	IsClosed  bool   `json:"is_closed"`
	SymbolID  uint32 `json:"symbol_id,omitempty"`
	Timeframe string `json:"timeframe,omitempty"`
}

// CandlesResult holds candle rows.
type CandlesResult struct {
	SymbolID         uint32   `json:"symbol_id,omitempty"`
	Timeframe        string   `json:"timeframe,omitempty"`
	Candles          []Candle `json:"candles"`
	ReferenceCandles []Candle `json:"reference_candles,omitempty"`
	NextPageToken    string   `json:"next_page_token,omitempty"`
}

// MarketTrade is a public market trade.
type MarketTrade struct {
	SymbolID uint32     `json:"symbol_id,omitempty"`
	MatchID  string     `json:"match_id,omitempty"`
	Price    PriceTicks `json:"price,omitempty"`
	Qty      QtyScaled  `json:"qty,omitempty"`
	TsNs     string     `json:"ts_ns,omitempty"`
	Side     string     `json:"side,omitempty"`
}

// MarketTradesResult holds public trades.
//
// CompleteThroughTradeID, NextAfterTradeID and EpochEnds are set only by
// MarketData.GetTradesAfter (gap-free backfill), where both watermarks are
// always decimal match IDs, "0" included; trade IDs are formatted as in
// MarketTrade.MatchID.
type MarketTradesResult struct {
	Trades        []MarketTrade `json:"trades"`
	NextPageToken string        `json:"next_page_token,omitempty"`
	// CompleteThroughTradeID: every trade of the symbol with a match ID at
	// or below it exists and is returned by the backfill. It only grows.
	CompleteThroughTradeID string `json:"complete_through_trade_id,omitempty"`
	// NextAfterTradeID is the afterTradeID of the next call; it never moves
	// backwards. Equal to the request's, nothing newer is available yet:
	// retry later with backoff.
	NextAfterTradeID string `json:"next_after_trade_id,omitempty"`
	// EpochEnds lists, ascending, every epoch that ended between the
	// request's afterTradeID and NextAfterTradeID, both inclusive.
	EpochEnds []TradeEpochEnd `json:"epoch_ends,omitempty"`
}

// TradeEpochEnd marks the end of a trade epoch in a gap-free backfill.
type TradeEpochEnd struct {
	Epoch uint64 `json:"epoch"`
	// LastTradeID is the match ID of the epoch's last trade, or
	// (epoch-1) * 2^40 when the epoch ended without trades.
	LastTradeID string `json:"last_trade_id"`
}

// MarketOverviewEntry is one market overview row.
type MarketOverviewEntry struct {
	SymbolID             uint32     `json:"symbol_id"`
	Symbol               string     `json:"symbol,omitempty"`
	LastPrice            PriceTicks `json:"last_price,omitempty"`
	IndexPrice           PriceTicks `json:"index_price,omitempty"`
	Volume24HBaseScaled  *string    `json:"volume_24h_base_scaled,omitempty"`
	Volume24HBase        *string    `json:"volume_24h_base,omitempty"`
	Volume24HQuoteScaled *string    `json:"volume_24h_quote_scaled,omitempty"`
	Volume24HUsdScaled   *string    `json:"volume_24h_usd_scaled,omitempty"`
	// Change24hBps is the change of the last price over 24 hours, in basis points.
	Change24hBps int32 `json:"change_24h_bps"`
	// Sparklines holds the sparklines the server sent, only when
	// includeSparklines was requested.
	Sparklines []MarketOverviewSparkline `json:"sparklines,omitempty"`
	// High24h and Low24h are the highest and lowest trade prices over the last
	// 24 hours; nil when the server sent none.
	High24h *PriceTicks `json:"high_24h,omitempty"`
	Low24h  *PriceTicks `json:"low_24h,omitempty"`
	// BestBid and BestBidQty are the best bid price and the base quantity at
	// it, at the pair's base quantity scale when the catalogs know it; both
	// nil when the bid side of the book is empty.
	BestBid    *PriceTicks `json:"best_bid,omitempty"`
	BestBidQty *QtyScaled  `json:"best_bid_qty,omitempty"`
	// BestAsk and BestAskQty are the best ask price and the base quantity at
	// it; both nil when the ask side of the book is empty.
	BestAsk    *PriceTicks `json:"best_ask,omitempty"`
	BestAskQty *QtyScaled  `json:"best_ask_qty,omitempty"`
	// LastTradeTsNs is the time of the last trade, in epoch nanoseconds;
	// empty when there was none.
	LastTradeTsNs string `json:"last_trade_ts_ns,omitempty"`
	// ListedTsNs is the time the market was listed, in epoch nanoseconds;
	// empty when unknown.
	ListedTsNs string `json:"listed_ts_ns,omitempty"`
	// PriceBand is the pair's live price band, a convenience copy of what
	// GetPriceBand returns. Nil means no fresh band is known, not that the
	// pair has no band; a band past its ValidUntilTsNs is unknown too (see
	// LivePriceBand.FreshAt). The overview stream sends a row again when its
	// band changes, appears or lapses, not on each confirmation. To check an
	// order price, read GetPriceBand or the price band stream.
	PriceBand *LivePriceBand `json:"price_band,omitempty"`
}

// LivePriceBand is a pair's live price band. Prices are Q9 price ticks.
type LivePriceBand struct {
	SymbolID uint32 `json:"symbol_id"`
	// State is "active" (at least one price lies inside the band), "off"
	// (the pair applies no band), "empty" (the band is on and no price lies
	// inside it: every order that adds exposure is refused) or "unknown" (no
	// band was confirmed current within the last 5 seconds).
	State string `json:"state"`
	// ReferencePrice is set when State is "active" or "empty".
	ReferencePrice *PriceTicks `json:"reference_price,omitempty"`
	// ReferenceSource is "last_trade", "listing_reference" or "index_price";
	// empty when State is "off" or "unknown".
	ReferenceSource string `json:"reference_source,omitempty"`
	// MaxBuyPrice and MinSellPrice are the inclusive limits for buys and
	// sells, set only when State is "active".
	MaxBuyPrice  *PriceTicks `json:"max_buy_price,omitempty"`
	MinSellPrice *PriceTicks `json:"min_sell_price,omitempty"`
	// MinBuyPrice and MaxSellPrice are set only when the band limits passive
	// prices too.
	MinBuyPrice  *PriceTicks `json:"min_buy_price,omitempty"`
	MaxSellPrice *PriceTicks `json:"max_sell_price,omitempty"`
	// UpdatedTsNs is when the band last changed, in epoch nanoseconds.
	UpdatedTsNs uint64 `json:"updated_ts_ns,omitempty"`
	// ReanchorEligibleTsNs is from when the next order operation may move the
	// reference price; nil when State is not "active" or "empty".
	ReanchorEligibleTsNs *uint64 `json:"reanchor_eligible_ts_ns,omitempty"`
	ReanchorPending      bool    `json:"reanchor_pending,omitempty"`
	// Parameters are the band's settings, as SpotPair.PriceBand; nil when
	// State is "off" or they cannot be expressed exactly.
	Parameters *PriceBand `json:"parameters,omitempty"`
	// Epoch never decreases; UpdateID orders bands within one epoch.
	Epoch    uint64 `json:"epoch,omitempty"`
	UpdateID uint64 `json:"update_id,omitempty"`
	// AsOfTsNs is when the band was last confirmed current, and
	// ValidUntilTsNs 5 seconds later, in epoch nanoseconds.
	AsOfTsNs       uint64 `json:"as_of_ts_ns,omitempty"`
	ValidUntilTsNs uint64 `json:"valid_until_ts_ns,omitempty"`
}

// FreshAt reports whether b is a band known to be current at now: present,
// not "unknown", and not past its ValidUntilTsNs. A band that is not fresh
// means no band is known, never that the pair has none.
func (b *LivePriceBand) FreshAt(now time.Time) bool {
	if b == nil || b.State == "unknown" || b.State == "" {
		return false
	}
	nanos := now.UnixNano()
	return nanos >= 0 && uint64(nanos) <= b.ValidUntilTsNs
}

// MarketOverviewSparkline is a market's recent closing prices over Interval,
// newest first.
type MarketOverviewSparkline struct {
	// Interval is SPARKLINE_1H, SPARKLINE_24H, SPARKLINE_1W or SPARKLINE_1M.
	Interval    string       `json:"interval"`
	ClosePrices []PriceTicks `json:"close_prices"`
}

// MarketOverviewList holds overview rows.
type MarketOverviewList struct {
	Markets       []MarketOverviewEntry `json:"markets"`
	NextPageToken string                `json:"next_page_token,omitempty"`
}

// SpotPairVolumeSeries is one pair's trailing-24h USD volume samples.
type SpotPairVolumeSeries struct {
	SymbolID        uint32  `json:"symbol_id"`
	Symbol          string  `json:"symbol,omitempty"`
	VolumeUsdScaled []int64 `json:"volume_usd_scaled"`
}

// SpotVolumeHistory is the columnar GetSpotVolumeHistory response.
type SpotVolumeHistory struct {
	Bucket               string                 `json:"bucket,omitempty"`
	StartTsSec           uint32                 `json:"start_ts_sec,omitempty"`
	EndTsSec             uint32                 `json:"end_ts_sec,omitempty"`
	Points               uint32                 `json:"points,omitempty"`
	Pairs                []SpotPairVolumeSeries `json:"pairs"`
	TotalVolumeUsdScaled []int64                `json:"total_volume_usd_scaled"`
}

// CurrencyMetadata is display metadata for one fiat currency or stablecoin.
type CurrencyMetadata struct {
	Code               string `json:"code,omitempty"`
	DefaultEnglishName string `json:"default_english_name,omitempty"`
	Symbol             string `json:"symbol,omitempty"`
	FractionDigits     uint32 `json:"fraction_digits,omitempty"`
}

// CurrencyConversionConfig is supported fiat and stablecoin display metadata.
type CurrencyConversionConfig struct {
	Fiat        []CurrencyMetadata `json:"fiat"`
	Stablecoins []CurrencyMetadata `json:"stablecoins"`
}

// FiatConversionRate is fiat currency units per 1 USD at 1e8 scale.
type FiatConversionRate struct {
	Code          string `json:"code,omitempty"`
	UnitsPerUsdE8 int64  `json:"units_per_usd_e8,omitempty"`
}

// FiatConversionSnapshot is one complete fiat observation.
type FiatConversionSnapshot struct {
	Rates       []FiatConversionRate `json:"rates"`
	SourceTsSec uint64               `json:"source_ts_sec,omitempty"`
	Stale       bool                 `json:"stale,omitempty"`
}

// StablecoinConversionRate is observed USD per stablecoin unit at 1e8 scale.
type StablecoinConversionRate struct {
	Code         string `json:"code,omitempty"`
	UsdPerUnitE8 int64  `json:"usd_per_unit_e8,omitempty"`
	SourceTsSec  uint64 `json:"source_ts_sec,omitempty"`
	Stale        bool   `json:"stale,omitempty"`
}

// CurrencyConversionRates groups fiat and stablecoin conversion observations.
type CurrencyConversionRates struct {
	Fiat          *FiatConversionSnapshot    `json:"fiat,omitempty"`
	Stablecoins   []StablecoinConversionRate `json:"stablecoins"`
	SnapshotTsSec uint64                     `json:"snapshot_ts_sec,omitempty"`
}
