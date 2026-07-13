package models

// Candle is one OHLCV candle.
type Candle struct {
	TsSec     int64  `json:"ts_sec,omitempty"`
	Open      string `json:"open,omitempty"`
	High      string `json:"high,omitempty"`
	Low       string `json:"low,omitempty"`
	Close     string `json:"close,omitempty"`
	Volume    string `json:"volume,omitempty"`
	SymbolID  uint32 `json:"symbol_id,omitempty"`
	Timeframe string `json:"timeframe,omitempty"`
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
	Trades []MarketTrade `json:"trades"`
}

// MarketOverviewEntry is one market overview row.
type MarketOverviewEntry struct {
	SymbolID  uint32     `json:"symbol_id"`
	Symbol    string     `json:"symbol,omitempty"`
	LastPrice PriceTicks `json:"last_price,omitempty"`
}

// MarketOverviewList holds overview rows.
type MarketOverviewList struct {
	Markets       []MarketOverviewEntry `json:"markets"`
	NextPageToken string                `json:"next_page_token,omitempty"`
}
