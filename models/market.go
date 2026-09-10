package models

// Candle is one OHLCV candle.
type Candle struct {
	TsSec       int64  `json:"ts_sec,omitempty"`
	Open        string `json:"open,omitempty"`
	High        string `json:"high,omitempty"`
	Low         string `json:"low,omitempty"`
	Close       string `json:"close,omitempty"`
	Volume      string `json:"volume,omitempty"`
	QuoteVolume string `json:"quote_volume,omitempty"`
	SymbolID    uint32 `json:"symbol_id,omitempty"`
	Timeframe   string `json:"timeframe,omitempty"`
}

// CandlesResult holds candle rows.
type CandlesResult struct {
	SymbolID      uint32   `json:"symbol_id,omitempty"`
	Timeframe     string   `json:"timeframe,omitempty"`
	Candles       []Candle `json:"candles"`
	NextPageToken string   `json:"next_page_token,omitempty"`
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
type MarketTradesResult struct {
	Trades        []MarketTrade `json:"trades"`
	NextPageToken string        `json:"next_page_token,omitempty"`
}

// MarketOverviewEntry is one market overview row.
type MarketOverviewEntry struct {
	SymbolID             uint32     `json:"symbol_id"`
	Symbol               string     `json:"symbol,omitempty"`
	LastPrice            PriceTicks `json:"last_price,omitempty"`
	IndexPrice           PriceTicks `json:"index_price,omitempty"`
	Volume24HBaseScaled  *string    `json:"volume_24h_base_scaled,omitempty"`
	Volume24HQuoteScaled *string    `json:"volume_24h_quote_scaled,omitempty"`
	Volume24HUsdScaled   *string    `json:"volume_24h_usd_scaled,omitempty"`
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
