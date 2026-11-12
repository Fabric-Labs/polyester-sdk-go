package models

import "time"

// CreateOrderRequest is the SDK input for orders.create.
type CreateOrderRequest struct {
	Symbol    *string `json:"symbol,omitempty"`
	SymbolID  *uint32 `json:"symbol_id,omitempty"`
	Side      string  `json:"side"`
	OrderType string  `json:"order_type"`
	TIF       *string `json:"tif,omitempty"`
	// Qty is the requested base quantity. Set exactly one of Qty,
	// MaxQuoteDebitScaled or QuoteProceedsScaled.
	Qty QtyInput `json:"qty"`
	// MaxQuoteDebitScaled is the hard all-in quote debit budget. Construct with
	// QtyFromQuoteDecimal / QtyFromQuoteScaled (OrderQuote domain). The SDK
	// validates the embedded scale against the pair's catalog quote_quantity_scale.
	// Valid for BUY market and limit-IOC orders only.
	MaxQuoteDebitScaled QtyInput `json:"max_quote_debit_scaled"`
	// QuoteProceedsScaled sizes a SELL by the quote it should receive ("sell
	// 100 USDT worth"): the order sells whole quantity steps until its
	// proceeds before fees reach the target, and never more. The quote fee is
	// charged on those proceeds. Construct it like MaxQuoteDebitScaled
	// (QtyFromQuoteDecimal / QtyFromQuoteScaled, the pair's
	// quote_quantity_scale). Valid for SELL market and limit-IOC orders only,
	// without a Peg. The order holds the base the target sells at its limit
	// price, or a market order's slippage bound, and is refused
	// ERROR_CODE_INSUFFICIENT_FUNDS when that base is not free, or
	// ERROR_CODE_QUOTE_SIZE_TOO_SMALL when the target sells less than the
	// pair's minimums. The API refuses it with attached risk.
	QuoteProceedsScaled QtyInput    `json:"quote_proceeds_scaled"`
	Price               *PriceInput `json:"price,omitempty"`
	// DisplayQty makes a limit GTC or GTD order an iceberg order: the book
	// shows at most this base quantity at a time, and once that part is fully
	// executed the next part is shown at the same price, behind the orders
	// already resting there. Nil (or unset) places a plain order. It is a
	// base quantity like Qty, at the pair's base_quantity_scale, and must be
	// positive; it is refused before sending on market, IOC and FOK orders.
	// The API checks the rest: below the order's quantity and a multiple of
	// the pair's step (ERROR_CODE_ICEBERG_DISPLAY_INVALID), at least the
	// pair's minimum quantity and the order's quantity divided by 100, rounded
	// up (ERROR_CODE_ICEBERG_DISPLAY_TOO_SMALL), and no attached risk
	// (ERROR_CODE_ICEBERG_NOT_ALLOWED). The display cannot be modified; a
	// replace keeps it.
	DisplayQty *QtyInput `json:"display_qty,omitempty"`
	// Peg selects a limit order's price once, at entry, from the best bid or
	// ask plus OffsetTicks; the price stays fixed afterwards. Price is still
	// required: it is the BUY cap or SELL floor, and a derived price beyond
	// it is refused ERROR_CODE_PEG_LIMIT_VIOLATION. An empty reference side
	// is refused ERROR_CODE_PEG_REFERENCE_UNAVAILABLE. A peg is refused before
	// sending on a market order, with quote sizing (MaxQuoteDebitScaled or
	// QuoteProceedsScaled) and with AttachedRisk. Read the order for the
	// selected price; the create result confirms admission only.
	Peg          *OrderPeg `json:"peg,omitempty"`
	SubAccountID *string   `json:"sub_account_id,omitempty"`
	// ClientOrderID is optional. Set a stable non-empty value so you can
	// reconcile after an ambiguous failure. Look up that id before creating
	// again; a second create with the same id is always a conflict if the
	// first attempt admitted.
	ClientOrderID *string `json:"client_order_id,omitempty"`
	PostOnly      bool    `json:"post_only,omitempty"`
	// ExpiresAt is required RFC3339 UTC when TIF is "gtd"; rejected otherwise.
	// Maps to proto LimitGtd.expire_at (1 second–30 days after validation).
	ExpiresAt *string `json:"expires_at,omitempty"`
	// AttachedRisk optionally attaches take-profit, stop-loss or trailing-stop
	// legs that arm after the order fills.
	AttachedRisk *AttachedRiskInput `json:"attached_risk,omitempty"`
	// MarketClientRefPrice is the client-supplied reference price for MARKET
	// orders. Rejected on limit orders.
	MarketClientRefPrice *PriceInput `json:"market_client_ref_price,omitempty"`
	// FeeAsset selects the fee asset: "quote" (default) or "base". Base fees
	// are valid only for BUY orders.
	FeeAsset *string `json:"fee_asset,omitempty"`
	// MaxSlippageTicks and MaxSlippageBps are mutually exclusive market-IOC
	// execution protections. Omit both to keep pair defaults. Rejected on
	// limit orders. MaxSlippageBps must be 1–10000; ticks must be positive.
	MaxSlippageTicks *int64 `json:"max_slippage_ticks,omitempty"`
	MaxSlippageBps   *int32 `json:"max_slippage_bps,omitempty"`
	// SelfTradePreventionMode is "expire_maker", "expire_taker",
	// "expire_both" or "decrement"; nil leaves the server default,
	// expire_maker. It also governs the order's attached take-profit,
	// stop-loss and trailing stop orders: those never trade with the order
	// they protect, and one that self-trade prevention expires against
	// another of the account's resting orders leaves its leg live (a
	// "fire_refused" trigger event with FireRefusalReason
	// "self_trade_prevented").
	//
	// With "decrement", an incoming order that would trade against a resting
	// order of the same root account (another sub-account included) trades
	// with neither: both are reduced by the quantity that would have traded,
	// and one left with nothing is canceled with CancelReason
	// "stp_decrement" (both, when the quantities are equal). The incoming
	// order's mode decides, so a resting order of any mode can be reduced or
	// canceled this way. A fill-or-kill order that would meet such an order
	// is refused ERROR_CODE_FOK_INSUFFICIENT_LIQUIDITY.
	SelfTradePreventionMode *string `json:"self_trade_prevention_mode,omitempty"`
}

// OrderPeg is a limit order's static entry peg: the order is priced once, at
// entry, at the Reference side's best price plus OffsetTicks.
type OrderPeg struct {
	// Reference is "best_bid" or "best_ask" (any case).
	Reference string `json:"reference"`
	// OffsetTicks is a signed number of the pair's configured price
	// increments (its tick size, not 1e-9 price ticks) added to the reference
	// price: -2 on "best_bid" prices two increments below the best bid. It
	// counts price increments, not occupied book levels. Zero joins the
	// reference.
	OffsetTicks int64 `json:"offset_ticks,omitempty"`
}

// BatchReplaceItem is one item in orders.batch_replace.
type BatchReplaceItem struct {
	Key              OrderKey    `json:"key"`
	NewPrice         *PriceInput `json:"new_price,omitempty"`
	NewQty           *QtyInput   `json:"new_qty,omitempty"`
	NewClientOrderID *string     `json:"new_client_order_id,omitempty"`
	// NewAttachedRisk replaces the order's attached risk policy. A positive
	// trailing stop LimitPrice is sent as given: the server applies the
	// parent-side rule and answers ERROR_CODE_BAD_PRICE with no violation. A zero
	// or negative one is refused before sending, as on
	// models.TrailingStopInput.
	NewAttachedRisk *AttachedRiskInput `json:"new_attached_risk,omitempty"`
}

// BatchCancelItem is one item in orders.batch_cancel.
type BatchCancelItem struct {
	Key      OrderKey `json:"key"`
	SymbolID *uint32  `json:"symbol_id,omitempty"`
}

// OrderbookLevel is one orderbook price level.
type OrderbookLevel struct {
	Price PriceTicks `json:"price"`
	Qty   QtyScaled  `json:"qty"`
}

// OrderbookData is a normalized orderbook snapshot.
type OrderbookData struct {
	Symbol  string           `json:"symbol"`
	Depth   int              `json:"depth"`
	BookSeq string           `json:"book_seq"`
	Bids    []OrderbookLevel `json:"bids"`
	Asks    []OrderbookLevel `json:"asks"`
	// Available is true when the book is current: Bids and Asks are its
	// levels and an empty side means no liquidity. False while the book is
	// being built or after it was invalidated: its levels are unknown, not
	// empty. Request the snapshot again later.
	Available bool `json:"available"`
}

// ApiData is an escape hatch for responses not yet modeled.
type ApiData struct {
	Raw map[string]any `json:"raw"`
}

// SpotConfig is the venue's spot asset and pair catalog.
type SpotConfig struct {
	// Assets ordered by asset code.
	Assets []SpotAsset `json:"assets"`
	// Pairs ordered by symbol.
	Pairs []SpotPair `json:"pairs"`
	// MaxOpenOrdersTotal is the venue's default limit on resting orders
	// (good-till-cancelled and good-till-date) a typical account may hold
	// across all pairs together. An account may have a lower limit of its own.
	// Orders past it are rejected with ERROR_CODE_MAX_OPEN_ORDERS_TOTAL. Zero
	// when not published.
	MaxOpenOrdersTotal uint32 `json:"max_open_orders_total"`
	// TsSec is the snapshot generation time in Unix seconds.
	TsSec uint64 `json:"ts_sec"`
	// Raw is the proto-JSON payload that Catalogs.HydrateSpotConfig accepts.
	Raw map[string]any `json:"raw"`
}

// SpotAsset describes one spot asset.
type SpotAsset struct {
	Asset                   string `json:"asset"`
	LedgerID                uint32 `json:"ledger_id"`
	Name                    string `json:"name"`
	QuantityDisplayDecimals uint32 `json:"quantity_display_decimals"`
	// QuantityScale is the decimal scale of this asset's quantities and
	// balances.
	QuantityScale int `json:"quantity_scale"`
	// MarketDataVolumeScale is the scale of public candle and market-overview
	// base volume.
	MarketDataVolumeScale int `json:"market_data_volume_scale"`
	// IncludedInEquity says whether the asset counts in the account's total
	// equity. Nil means unknown (a server that predates the field), never
	// false: value only assets where it is true, and show no value for an
	// asset without it. An asset outside equity can still have a market and a
	// live index price.
	IncludedInEquity *bool `json:"included_in_equity,omitempty"`
}

// SpotPair describes one spot trading pair and its order rules. Decimal
// strings are in base-asset (quantity) or quote-asset (notional) units.
type SpotPair struct {
	SymbolID            uint32 `json:"symbol_id"`
	Symbol              string `json:"symbol"`
	BaseAsset           string `json:"base_asset"`
	QuoteAsset          string `json:"quote_asset"`
	TickSize            string `json:"tick_size"`
	StepSize            string `json:"step_size"`
	MinNotionalQuote    string `json:"min_notional_quote"`
	MinQtyBase          string `json:"min_qty_base"`
	AllowBuyFeeFromBase bool   `json:"allow_buy_fee_from_base"`
	BaseQuantityScale   int    `json:"base_quantity_scale"`
	QuoteQuantityScale  int    `json:"quote_quantity_scale"`
	// ReferencePriceScale is the scale of composite reference candle prices.
	// Trading prices use the fixed scale 9.
	ReferencePriceScale   int        `json:"reference_price_scale"`
	OrderbookPriceBuckets []float64  `json:"orderbook_price_buckets,omitempty"`
	ListingAt             *time.Time `json:"listing_at,omitempty"`
	DelistingAt           *time.Time `json:"delisting_at,omitempty"`
	// Status is enabled, disabled, cancel_only, post_only, reduce_only, halted,
	// or unknown.
	Status                       string `json:"status"`
	DefaultMarketSlippageBpsBuy  int32  `json:"default_market_slippage_bps_buy"`
	DefaultMarketSlippageBpsSell int32  `json:"default_market_slippage_bps_sell"`
	MaxClientRefDriftBps         int32  `json:"max_client_ref_drift_bps"`
	// MaxLimitQtyBase is the largest quantity of one limit order; a replace is
	// checked on its new total less the quantity already filled. Larger orders
	// are rejected with ERROR_CODE_MAX_QTY. Empty when the pair has no maximum.
	MaxLimitQtyBase string `json:"max_limit_qty_base,omitempty"`
	// MaxMarketQtyBase is the largest quantity of one market order, after a
	// quote-sized order is converted to base. Empty when the pair has no
	// maximum.
	MaxMarketQtyBase string `json:"max_market_qty_base,omitempty"`
	// MaxOrderNotionalQuote is the largest notional of one order: quantity
	// times limit price, or times protection price for a market order. Larger
	// orders are rejected with ERROR_CODE_MAX_NOTIONAL. Empty when the pair has
	// no maximum.
	MaxOrderNotionalQuote string `json:"max_order_notional_quote,omitempty"`
	// MaxOpenOrdersPerPair is the venue's default limit on resting orders a
	// typical account may hold on this pair. An account may have a lower
	// limit of its own. Orders past it are rejected with
	// ERROR_CODE_MAX_OPEN_ORDERS_PER_PAIR. Zero when not published.
	MaxOpenOrdersPerPair uint32 `json:"max_open_orders_per_pair"`
	// MaxMarketNotionalQuote is the largest notional of one market order in
	// quote units. A market order is valued at the top of its band: a buy at
	// its protection price, a sell at its reference price. Larger orders are
	// rejected with ERROR_CODE_MAX_NOTIONAL. Empty when the pair has no
	// maximum.
	MaxMarketNotionalQuote string `json:"max_market_notional_quote,omitempty"`
	// PriceBand limits how far from the market an order may be priced or
	// trade on this pair. Nil when the pair has no band.
	PriceBand *PriceBand `json:"price_band,omitempty"`
}

// PriceBand is a pair's price band. Its limits sit LowerBps below and
// UpperBps above a reference price R that follows the pair's last trade,
// moves to the index price when the two diverge by more than the band, and
// stays fixed for ReferenceWindowMs before it can move to a newer trade. With
// tick size T the limits are ceil(R × (1 − LowerBps/10000) / T) × T and
// floor(R × (1 + UpperBps/10000) / T) × T. R is not published; the last trade
// price is a close estimate, so checking a limit price against
// last × (1 ± bps/10000) avoids most refusals.
//
// A buy limit priced above the upper limit, or a sell limit below the lower
// one, is refused ERROR_CODE_PRICE_BAND_VIOLATION, as is a replace that adds
// exposure beyond it (reducing quantity is always allowed). A market order,
// or another order the exchange prices, trades only up to the limit and its
// unfilled rest is canceled with CancelReason "price_band". The band never
// cancels a resting order. An attached take-profit, stop-loss or trailing
// stop whose order the band refuses stays active and is sent again once the
// band moves; a standalone trigger fails with FailureReason
// "price_band_violation".
type PriceBand struct {
	LowerBps          uint32 `json:"lower_bps"`
	UpperBps          uint32 `json:"upper_bps"`
	ReferenceWindowMs uint32 `json:"reference_window_ms"`
}

// PairConstraints is the deterministic, parsed trading-rule view for one spot
// pair. Decimal strings preserve the catalog contract; scaled fields are ready
// for local inspection. The API remains authoritative for order admission.
type PairConstraints struct {
	Symbol                string `json:"symbol"`
	SymbolID              uint32 `json:"symbol_id"`
	BaseQuantityScale     int    `json:"base_quantity_scale"`
	QuoteQuantityScale    int    `json:"quote_quantity_scale"`
	TickSize              string `json:"tick_size"`
	TickSizeTicks         int64  `json:"tick_size_ticks"`
	StepSize              string `json:"step_size"`
	StepSizeScaled        int64  `json:"step_size_scaled"`
	MinQtyBase            string `json:"min_qty_base"`
	MinQtyScaled          int64  `json:"min_qty_scaled"`
	MinNotionalQuote      string `json:"min_notional_quote"`
	MinNotionalComputable bool   `json:"min_notional_computable"`
	// MaxLimitQtyBase is the largest quantity of one limit order; empty (and
	// MaxLimitQtyScaled zero) when the pair has no maximum.
	MaxLimitQtyBase   string `json:"max_limit_qty_base,omitempty"`
	MaxLimitQtyScaled int64  `json:"max_limit_qty_scaled,omitempty"`
	// MaxMarketQtyBase is the largest quantity of one market order; empty (and
	// MaxMarketQtyScaled zero) when the pair has no maximum.
	MaxMarketQtyBase   string `json:"max_market_qty_base,omitempty"`
	MaxMarketQtyScaled int64  `json:"max_market_qty_scaled,omitempty"`
	// MaxOrderNotionalQuote is the largest notional of one order in quote
	// units; empty when the pair has no maximum.
	MaxOrderNotionalQuote string `json:"max_order_notional_quote,omitempty"`
	// MaxMarketNotionalQuote is the largest notional of one market order in
	// quote units, valued at the top of its band; empty when the pair has no
	// maximum.
	MaxMarketNotionalQuote string `json:"max_market_notional_quote,omitempty"`
	// MaxOpenOrdersPerPair is the venue's default resting-order limit for a
	// typical account on this pair; zero when not published. An account may
	// have a lower limit of its own.
	MaxOpenOrdersPerPair uint32 `json:"max_open_orders_per_pair,omitempty"`
	// MaxQuoteSlippage is the largest absolute max-slippage an order accepts,
	// in quote units: the int64 ceiling of Q9 price ticks.
	MaxQuoteSlippage      string `json:"max_quote_slippage"`
	MaxQuoteSlippageTicks int64  `json:"max_quote_slippage_ticks"`
	// PriceBand is the pair's price band, as SpotPair.PriceBand; nil when the
	// pair has none.
	PriceBand *PriceBand `json:"price_band,omitempty"`
}
