package models

import "time"

// Profit and loss figures are in USD. Every amount is an exact base-10
// decimal string, signed where its field says so: parse it with a decimal
// type, never a float. An optional figure (a *string) is nil when a price it
// needs is not available, which is distinct from "0"; Notices then says what
// the figures leave out.
//
// Notices hold the contract's ProfitAndLossNotice values without their
// prefix: "price_unavailable", "mark_unavailable", "unclassified_transfer",
// "quantity_shortfall", "equity_unavailable", "flow_unvalued" and
// "sharpe_insufficient_days"; a value newer than this SDK reads
// "unknown(N)". Products read "spot".

// ProfitAndLoss is the account's profit and loss over a window, by product
// and market, with the assets it holds now.
type ProfitAndLoss struct {
	// WindowStart is the start of the window (UTC), inclusive.
	WindowStart *time.Time `json:"window_start,omitempty"`
	// AsOf is when the figures were last published. Trades and transfers
	// settled shortly before may not be included yet.
	AsOf *time.Time `json:"as_of,omitempty"`
	// MarkTime is the time of the prices that value holdings now.
	MarkTime *time.Time `json:"mark_time,omitempty"`
	// PnLUSD is RealizedPnLUSD plus the change in unrealized PnL over the
	// window, with deposits, withdrawals and transfers neutral.
	PnLUSD *string `json:"pnl_usd,omitempty"`
	// RealizedPnLUSD is the realized PnL of the window's trades, after fees.
	RealizedPnLUSD string `json:"realized_pnl_usd"`
	// UnrealizedPnLChangeUSD is the change in unrealized PnL from the
	// window's start to now, net of transfers.
	UnrealizedPnLChangeUSD *string `json:"unrealized_pnl_change_usd,omitempty"`
	// UnrealizedPnLUSD is the unrealized PnL of the holdings now.
	UnrealizedPnLUSD *string `json:"unrealized_pnl_usd,omitempty"`
	// FeesUSD is the fees paid net of rebates; negative when rebates exceed
	// fees.
	FeesUSD string `json:"fees_usd"`
	// VolumeUSD is the traded volume, non-negative.
	VolumeUSD string                 `json:"volume_usd"`
	Products  []ProductProfitAndLoss `json:"products"`
	// Markets is the realized side per market, ordered by symbol ID.
	Markets []MarketProfitAndLoss `json:"markets"`
	// Holdings are the assets held now, ordered by asset ID.
	Holdings []HoldingProfitAndLoss `json:"holdings"`
	Notices  []string               `json:"notices,omitempty"`
}

// ProductProfitAndLoss is the PnL of one product over the window.
type ProductProfitAndLoss struct {
	Product        string `json:"product"`
	RealizedPnLUSD string `json:"realized_pnl_usd"`
	FeesUSD        string `json:"fees_usd"`
	VolumeUSD      string `json:"volume_usd"`
}

// MarketProfitAndLoss is the realized side of one market over the window.
type MarketProfitAndLoss struct {
	SymbolID       uint32 `json:"symbol_id"`
	Product        string `json:"product"`
	RealizedPnLUSD string `json:"realized_pnl_usd"`
	FeesUSD        string `json:"fees_usd"`
	VolumeUSD      string `json:"volume_usd"`
	// TradeCount counts the account's fills in the market.
	TradeCount uint64 `json:"trade_count"`
}

// HoldingProfitAndLoss is one asset the account holds now. Quantities are in
// whole units of the asset.
type HoldingProfitAndLoss struct {
	AssetID uint32 `json:"asset_id"`
	// PositionQuantity is the quantity held: the quantity with a known cost
	// plus UntrackedQuantity.
	PositionQuantity string `json:"position_quantity"`
	// UntrackedQuantity has no known cost, such as deposited coins, and
	// never counts toward PnL.
	UntrackedQuantity string `json:"untracked_quantity"`
	// CostUSD is the cost of the quantity with a known cost.
	CostUSD string `json:"cost_usd"`
	// EntryPriceUSD is the average entry price per unit; nil when no
	// quantity has a known cost.
	EntryPriceUSD *string `json:"entry_price_usd,omitempty"`
	// MarkPriceUSD is the current price per unit; nil when not available.
	MarkPriceUSD *string `json:"mark_price_usd,omitempty"`
	// UnrealizedPnLUSD is nil when MarkPriceUSD is.
	UnrealizedPnLUSD *string `json:"unrealized_pnl_usd,omitempty"`
}

// DailyProfitAndLoss is one UTC day of an account.
type DailyProfitAndLoss struct {
	// Day is the day at 00:00 UTC.
	Day *time.Time `json:"day,omitempty"`
	// PnLUSD is RealizedPnLUSD plus the change in unrealized PnL from the
	// previous day's close, with transfers neutral.
	PnLUSD         *string `json:"pnl_usd,omitempty"`
	RealizedPnLUSD string  `json:"realized_pnl_usd"`
	// UnrealizedPnLAtCloseUSD is nil when a price it needs is not available,
	// as for today before it closes.
	UnrealizedPnLAtCloseUSD *string  `json:"unrealized_pnl_at_close_usd,omitempty"`
	FeesUSD                 string   `json:"fees_usd"`
	VolumeUSD               string   `json:"volume_usd"`
	TradeCount              uint64   `json:"trade_count"`
	Notices                 []string `json:"notices,omitempty"`
	// StartEquityUSD is the account's equity at the 00:00 UTC that starts
	// the day: everything it owns, reserved funds included. It equals the
	// previous day's EndEquityUSD.
	StartEquityUSD *string `json:"start_equity_usd,omitempty"`
	// EndEquityUSD is the equity at the 00:00 UTC that ends the day; nil for
	// today until the day closes.
	EndEquityUSD *string `json:"end_equity_usd,omitempty"`
	// NetFlowUSD is deposits and transfers in minus withdrawals and
	// transfers out, each valued when it settled.
	NetFlowUSD *string `json:"net_flow_usd,omitempty"`
}

// DailyProfitAndLossList is one page of days, oldest first. Every day of the
// range is listed, days without activity included.
type DailyProfitAndLossList struct {
	Days []DailyProfitAndLoss `json:"days"`
	// NextPageToken continues the list; empty on the last page.
	NextPageToken string     `json:"next_page_token"`
	AsOf          *time.Time `json:"as_of,omitempty"`
}

// TradingStatistics summarizes the account's trading over a window. The
// trade count, volume, fees and realized PnL cover the window, today
// included; the return figures cover closed UTC days only, for the whole
// account whatever symbol and product select.
type TradingStatistics struct {
	WindowStart    *time.Time `json:"window_start,omitempty"`
	AsOf           *time.Time `json:"as_of,omitempty"`
	TradeCount     uint64     `json:"trade_count"`
	VolumeUSD      string     `json:"volume_usd"`
	FeesUSD        string     `json:"fees_usd"`
	RealizedPnLUSD string     `json:"realized_pnl_usd"`
	Notices        []string   `json:"notices,omitempty"`
	// ReturnPct is the time-weighted return in percent ("1.25" is 1.25%),
	// deposits and withdrawals excluded; nil when a day's equity or net flow
	// is not available, or the window has no closed day (TODAY).
	ReturnPct *string `json:"return_pct,omitempty"`
	// MaxDrawdownPct is the largest fall in percent of the compounded return
	// from a previous peak, at day closes.
	MaxDrawdownPct *string `json:"max_drawdown_pct,omitempty"`
	// Sharpe is the annualized Sharpe ratio of the daily returns, a plain
	// ratio; nil below 7 daily returns (notice "sharpe_insufficient_days").
	Sharpe *string `json:"sharpe,omitempty"`
	// ReturnDays counts the daily returns the return figures are based on.
	ReturnDays uint32 `json:"return_days"`
}
