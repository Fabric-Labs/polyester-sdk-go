package models

import (
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// Order is a normalized open or historical order.
type Order struct {
	OrderID       string `json:"order_id"`
	SymbolID      uint32 `json:"symbol_id"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Side          string `json:"side,omitempty"`
	Status        string `json:"status,omitempty"`
	OrderType     string `json:"order_type,omitempty"`
	TIF           string `json:"tif,omitempty"`
	// OrigQty is the current accepted total quantity. A successful modify
	// updates it to the amended total; retain the first submitted quantity
	// separately if you need it. CumQty and AvgPx are cumulative execution
	// values across the lineage through the returned generation, not values
	// limited to one physical order. LeavesQty is remaining working quantity.
	OrigQty QtyScaled `json:"orig_qty,omitempty"`
	CumQty  QtyScaled `json:"cum_qty,omitempty"`
	// InheritedCumQty is the filled quantity a replacement inherited from its
	// predecessors when it was accepted; zero for an original order. CumQty
	// minus InheritedCumQty is what this generation executed.
	InheritedCumQty QtyScaled `json:"inherited_cum_qty,omitempty"`
	LeavesQty       QtyScaled `json:"leaves_qty,omitempty"`
	// DisplayQty is the iceberg display the order was placed with: the book
	// shows at most this much of it at a time. HiddenQty is the part of
	// LeavesQty the book does not show, so the book shows LeavesQty minus
	// HiddenQty. Both are zero for a plain order, and HiddenQty is zero once
	// the order has ended. Public market data shows only the quantity on the
	// book.
	DisplayQty  QtyScaled  `json:"display_qty,omitempty"`
	HiddenQty   QtyScaled  `json:"hidden_qty,omitempty"`
	Price       PriceTicks `json:"price,omitempty"`
	AvgPx       PriceTicks `json:"avg_px,omitempty"`
	CreatedTsNs string     `json:"created_ts_ns,omitempty"`
	// TerminalTsNs is when the order became terminal, in nanoseconds since
	// the Unix epoch (UTC) as a decimal string. Empty while the order is open.
	TerminalTsNs string `json:"terminal_ts_ns,omitempty"`
	Version      uint32 `json:"version,omitempty"`
	PostOnly     bool   `json:"post_only,omitempty"`
	FeeAsset     string `json:"fee_asset,omitempty"`
	// SelfTradePreventionMode is "expire_maker", "expire_taker",
	// "expire_both" or "decrement"; empty when the server does not report it, and
	// "unknown(N)" for a mode newer than this SDK.
	SelfTradePreventionMode string `json:"self_trade_prevention_mode,omitempty"`
	// SubmittedMaxQuoteDebitScaled is populated for a BUY sized by a quote
	// budget, at the pair's quote_quantity_scale.
	SubmittedMaxQuoteDebitScaled string `json:"submitted_max_quote_debit_scaled,omitempty"`
	// SubmittedQuoteProceedsScaled is the quote proceeds target of a SELL
	// sized by QuoteProceedsScaled, at the pair's quote_quantity_scale; empty
	// for every other order. For such an order CumNotional is the proceeds it
	// received before fees, so the target less CumQuote is what it did not
	// receive.
	SubmittedQuoteProceedsScaled string        `json:"submitted_quote_proceeds_scaled,omitempty"`
	AttachedRisk                 *AttachedRisk `json:"attached_risk,omitempty"`
	Lineage                      *OrderLineage `json:"lineage,omitempty"`
	// Origin is how the order was created: directly, or as the child of a
	// trigger or attached-risk leg. Nil when the server does not report it.
	Origin *OrderOrigin `json:"origin,omitempty"`
	// MarketClientRefPrice is the reference price a MARKET order's
	// slippage protection was measured from; nil when none was supplied.
	MarketClientRefPrice *PriceTicks `json:"market_client_ref_price,omitempty"`
	// MarketMaxSlippage (an absolute price distance) or MarketMaxSlippageBps
	// is a MARKET order's maximum slippage. Nil and zero when the order has
	// no cap of that form.
	MarketMaxSlippage    *PriceTicks `json:"market_max_slippage,omitempty"`
	MarketMaxSlippageBps int32       `json:"market_max_slippage_bps,omitempty"`
	// ExpireAt is the RFC3339 UTC expiry for GTD orders. Empty for every other TIF.
	ExpireAt string `json:"expire_at,omitempty"`
	// BatchRequestID is the public ID of the batch replace that admitted the
	// order, as BatchReplaceOrdersResult.BatchRequestID; empty for every other
	// origin.
	BatchRequestID string `json:"batch_request_id,omitempty"`
	// RejectReason is why a rejected order was refused, named as the order
	// write API names the same refusal (for example "POST_ONLY_CROSS"). Empty
	// for every other status.
	RejectReason string `json:"reject_reason,omitempty"`
	// CancelReason is why a canceled order ended (for example "user_request",
	// "replaced", or "attached_exit_fired": its take-profit, stop-loss or
	// trailing stop fired while it still rested, so its unfilled rest was
	// canceled first). "stp_decrement" is an order DECREMENT self-trade
	// prevention left with no quantity, "price_band" the unfilled rest of a
	// market (or exchange-priced) order the pair's price band stopped (its
	// fills stand), and "cancel_on_disconnect" an order canceled because the
	// account's opted-in trading WebSocket sessions all disconnected (its own
	// cancel-all-after timer reads "cancel_all_after"). Empty for every other
	// status.
	CancelReason string `json:"cancel_reason,omitempty"`
	// AvgPriceE18 is the exact average execution price across the lineage
	// through this generation, in quote units scaled by 1e18, as an unsigned
	// decimal integer string: the executed quote amount divided by CumQty,
	// rounded half to even. "0" with no fills. AvgPx is the same average
	// rounded down to 1e-9; prefer AvgPrice, which formats this one.
	AvgPriceE18 string `json:"avg_price_e18,omitempty"`
	// CumNotional is the exact quote amount of the fills across the lineage
	// through this generation, before fees, as an unsigned decimal integer
	// string in quote units scaled by 10^(9 + the pair's base_quantity_scale).
	// "0" with no fills. Format it with CumQuote. Each fill settles its quote
	// rounded down to 18 decimals, so CumNotional is exactly the settled quote
	// when base_quantity_scale is at most 9; above that it can exceed the
	// settled quote by less than 1e-18 per fill.
	CumNotional string `json:"cum_notional,omitempty"`
	// FeeTotal is the total fee of those fills, net of rebates, in FeeAsset.
	// Order history reads and single-order reads with execution history (the
	// default) carry it; open-order reads, single-order reads without
	// execution history and order stream updates leave it nil.
	FeeTotal *OrderFeeTotal `json:"fee_total,omitempty"`
}

// OrderOrigin is how an order was created.
//
// Scope is "direct", "attached_risk", "standalone_trigger" or "system", and
// "unknown(N)" for a scope newer than this SDK. TriggerType names the trigger
// strategy that placed the order ("stop_loss", "take_profit",
// "trailing_stop", "twap" or "ladder") and is empty for a direct order.
// TriggerID is that trigger, ParentOrderID the parent order of an
// attached-risk child, and ChildSeq the child's sequence number in a
// multi-child strategy such as a TWAP; each is empty or zero when it does not
// apply.
type OrderOrigin struct {
	Scope         string `json:"scope,omitempty"`
	TriggerType   string `json:"trigger_type,omitempty"`
	TriggerID     string `json:"trigger_id,omitempty"`
	ParentOrderID string `json:"parent_order_id,omitempty"`
	ChildSeq      uint32 `json:"child_seq,omitempty"`
}

// OrderFeeTotal is the net fee of an order's fills in the order's FeeAsset.
// A fill's fee is recorded shortly after the fill executes, so the total can
// trail the order's fills: it covers them all once Qty equals the order's
// CumQty.
type OrderFeeTotal struct {
	// AmountE18 is the net fee magnitude scaled by 1e18, as an unsigned
	// decimal integer string, with the same conversion as UserTrade.FeeAmountE18.
	AmountE18 string `json:"amount_e18"`
	// IsRebate is true when rebates exceed fees, so AmountE18 was credited.
	IsRebate bool `json:"is_rebate,omitempty"`
	// Qty is the executed quantity of the fills the total covers.
	Qty QtyScaled `json:"qty"`
}

// OrderLineage is the stable replacement-chain identity for an order or fill.
// ID is the first generation's public order ID. Generation is one-based.
type OrderLineage struct {
	ID         string `json:"id,omitempty"`
	Generation uint32 `json:"generation,omitempty"`
}

// OrderTransfer is a settlement leg linked to a match on an order or trades page.
// Identify a match by (SymbolID, MatchID). Deduplicate repeated legs by TxID.
type OrderTransfer struct {
	MatchID      string `json:"match_id,omitempty"`
	SymbolID     uint32 `json:"symbol_id,omitempty"`
	AssetID      uint32 `json:"asset_id,omitempty"`
	AmountE18    string `json:"amount_e18,omitempty"`
	IsDebit      bool   `json:"is_debit,omitempty"`
	TransferCode int32  `json:"transfer_code,omitempty"`
	AccountCode  int32  `json:"account_code,omitempty"`
	TsNs         string `json:"ts_ns,omitempty"`
	TxID         string `json:"tx_id,omitempty"`
}

// RiskLeg is a take-profit or stop-loss attached-risk projection. Policy
// fields remain zero when the response includes runtime state only.
// TriggerPriceSource is the price the leg watches ("last" or "index"; empty
// when the server did not name one, which watches the last traded price).
type RiskLeg struct {
	TriggerPrice       PriceTicks            `json:"trigger_price,omitempty"`
	TriggerPriceSource string                `json:"trigger_price_source,omitempty"`
	OrderType          string                `json:"order_type,omitempty"`
	LimitPrice         PriceTicks            `json:"limit_price,omitempty"`
	State              *AttachedRiskLegState `json:"state,omitempty"`
}

// AttachedRiskLegState is the runtime state and child linkage for one
// parent-order attached-risk leg. Nanosecond timestamps and public IDs use the
// same string representation as other trading read models.
type AttachedRiskLegState struct {
	Status       string `json:"status,omitempty"`
	ArmedTsNs    string `json:"armed_ts_ns,omitempty"`
	TerminalTsNs string `json:"terminal_ts_ns,omitempty"`
	TriggerID    string `json:"trigger_id,omitempty"`
	ChildOrderID string `json:"child_order_id,omitempty"`
}

// TrailingStop is a trailing-stop attached-risk policy projection.
//
// Decode preserves a leg when it has a positive trailing distance or meaningful
// runtime state; policy fields remain zero when only state was requested.
// TriggerPriceSource is the price the leg watches ("last" or "index"; empty
// when the server did not name one, which watches the last traded price).
//
// LimitPrice is the maximum price of a trailing stop that buys back, on an
// order that sells: the most its exit pays per unit, as the server returns it.
// It is nil when the server sends none, as for the trailing stop of an order
// that buys. When a trailing stop with a LimitPrice fires, its child is a limit
// IOC order at that price (lower when MaxSlippageTicks or MaxSlippageBps cap it
// below): it fills what it can at or below the limit and the rest is canceled,
// not left resting. Without a LimitPrice the child is a market order.
//
// OrderType is not on the trailing-stop wire and remains empty on decode.
// Write attached risk with AttachedRiskInput.
type TrailingStop struct {
	// DistanceTicks is the trailing distance as a price delta in 1e-9
	// quote-unit ticks (500_000_000 is 0.5 quote units).
	DistanceTicks      int64                 `json:"distance_ticks,omitempty"`
	DistanceBps        int32                 `json:"distance_bps,omitempty"`
	MaxSlippageTicks   int64                 `json:"max_slippage_ticks,omitempty"`
	MaxSlippageBps     int32                 `json:"max_slippage_bps,omitempty"`
	ActivationPrice    PriceTicks            `json:"activation_price,omitempty"`
	LimitPrice         *PriceTicks           `json:"limit_price,omitempty"`
	TriggerPriceSource string                `json:"trigger_price_source,omitempty"`
	OrderType          string                `json:"order_type,omitempty"`
	State              *AttachedRiskLegState `json:"state,omitempty"`
}

// AttachedRisk is the parent-side attached risk policy and per-leg runtime state.
type AttachedRisk struct {
	TakeProfit   *RiskLeg      `json:"take_profit,omitempty"`
	StopLoss     *RiskLeg      `json:"stop_loss,omitempty"`
	TrailingStop *TrailingStop `json:"trailing_stop,omitempty"`
	Oco          bool          `json:"oco,omitempty"`
}

// OrdersList holds paginated orders.
type OrdersList struct {
	Orders        []Order `json:"orders"`
	NextPageToken string  `json:"next_page_token,omitempty"`
}

// OrderMutationResult is the outcome of a single order mutation.
type OrderMutationResult struct {
	Status                       string     `json:"status"`
	OrderID                      string     `json:"order_id,omitempty"`
	ClientOrderID                string     `json:"client_order_id,omitempty"`
	ResolvedBaseQtyScaled        string     `json:"resolved_base_qty_scaled,omitempty"`
	ResolvedBaseQty              *QtyScaled `json:"resolved_base_qty,omitempty"`
	SubmittedMaxQuoteDebitScaled string     `json:"submitted_max_quote_debit_scaled,omitempty"`
	// SubmittedQuoteProceedsScaled echoes a SELL's quote proceeds target;
	// ResolvedBaseQty is then the base the order holds, the most it can sell.
	SubmittedQuoteProceedsScaled string `json:"submitted_quote_proceeds_scaled,omitempty"`
	AttachedRiskTriggerIDs
}

// AttachedRiskTriggerIDs are the trigger IDs of an order's attached risk legs,
// present only for configured legs.
type AttachedRiskTriggerIDs struct {
	TakeProfitTriggerID   string `json:"take_profit_trigger_id,omitempty"`
	StopLossTriggerID     string `json:"stop_loss_trigger_id,omitempty"`
	TrailingStopTriggerID string `json:"trailing_stop_trigger_id,omitempty"`
}

// OrderFieldViolation is one field-level validation failure on an order
// intent. It is errors.FieldViolation, the type order errors carry, so preview
// and batch rejections and refused requests read the same.
type OrderFieldViolation = errors.FieldViolation

// OrderErrorDetail is a typed order rejection with optional field violations.
type OrderErrorDetail struct {
	Code       string                `json:"code,omitempty"`
	Violations []OrderFieldViolation `json:"violations,omitempty"`
	// RateLimit is set when the rejection carries polyester.ratelimit.v1 detail.
	RateLimit *RateLimitDetail `json:"rate_limit,omitempty"`
}

// PreviewOrderResult is the advisory admission result of Orders.Preview.
// Fee/quote estimates are no longer returned on the wire; CreateOrder always
// re-evaluates the intent.
type PreviewOrderResult struct {
	Admissible            *bool             `json:"admissible,omitempty"`
	Rejection             *OrderErrorDetail `json:"rejection,omitempty"`
	ResolvedBaseQtyScaled string            `json:"resolved_base_qty_scaled,omitempty"`
	ResolvedBaseQty       *QtyScaled        `json:"resolved_base_qty,omitempty"`
	ProtectedPriceBound   *PriceTicks       `json:"protected_price_bound,omitempty"`
	EvaluatedAtMs         int64             `json:"evaluated_at_ms,omitempty"`
}

// GetOrderResult includes order detail and a page of lineage executions.
//
// The order shows a fill as soon as it executes; its trades are recorded
// shortly after. ExecutionsComplete is true when the order's trades across
// every page add up to its filled quantity. While it is false a recent
// execution is still being recorded: Trades lists those recorded so far and a
// later read returns the rest. It is always false when execution history is
// not included.
type GetOrderResult struct {
	Order              *Order          `json:"order,omitempty"`
	Trades             []UserTrade     `json:"trades,omitempty"`
	Transfers          []OrderTransfer `json:"transfers,omitempty"`
	NextPageToken      string          `json:"next_page_token,omitempty"`
	ExecutionsComplete bool            `json:"executions_complete,omitempty"`
}

// GetOrderOptions are optional GetOrder pagination and history flags.
// Execution history defaults to enabled on the server when IncludeExecutionHistory is nil.
type GetOrderOptions struct {
	IncludeExecutionHistory *bool
	Limit                   *uint32
	PageToken               *string
}

// UserTrade is a user fill record.
type UserTrade struct {
	SymbolID               uint32     `json:"symbol_id"`
	MatchID                string     `json:"match_id,omitempty"`
	OrderID                string     `json:"order_id,omitempty"`
	Side                   string     `json:"side,omitempty"`
	IsMaker                bool       `json:"is_maker,omitempty"`
	Price                  PriceTicks `json:"price,omitempty"`
	Qty                    QtyScaled  `json:"qty,omitempty"`
	FeeAmountE18           string     `json:"fee_amount_e18,omitempty"`
	FeeAsset               string     `json:"fee_asset,omitempty"`
	ReferralShareAmountE18 string     `json:"referral_share_amount_e18,omitempty"`
	TsNs                   string     `json:"ts_ns,omitempty"`
	// FeeIsRebate is true when fee_amount_e18 is a rebate credit instead of a fee debit.
	// Proto3 omits false, so sparse wire encoding only sets this when a rebate was earned.
	FeeIsRebate bool          `json:"fee_is_rebate,omitempty"`
	Lineage     *OrderLineage `json:"lineage,omitempty"`
}

// UserTradesList holds paginated user trades.
type UserTradesList struct {
	Trades        []UserTrade     `json:"trades"`
	Transfers     []OrderTransfer `json:"transfers,omitempty"`
	NextPageToken string          `json:"next_page_token,omitempty"`
}

// ListTriggersOptions are optional Triggers.List filters, as ListTriggers
// takes them. Empty or nil sends no filter.
type ListTriggersOptions struct {
	// TriggerType lists one trigger type: stop_loss, take_profit,
	// trailing_stop, twap or ladder.
	TriggerType string
	// ParentOrderID lists the attached triggers of one parent order.
	ParentOrderID *string
	// OrderListID lists the take-profit and the stop-loss of one OCO, by the
	// OrderListID CreateOco returned; ClientListID by its client OCO ID.
	OrderListID  *string
	ClientListID *string
}

// ListUserTradesOptions are optional GetUserTrades execution-scope filters.
// OrderID and LineageID are mutually exclusive. ThroughGeneration requires LineageID.
type ListUserTradesOptions struct {
	OrderID           *string
	LineageID         *string
	ThroughGeneration *uint32
	IncludeTransfers  bool
}

// ModifyOrderResult is returned from order modify RPCs.
type ModifyOrderResult struct {
	ActionTaken  string `json:"action_taken,omitempty"`
	OldOrderID   string `json:"old_order_id,omitempty"`
	FinalOrderID string `json:"final_order_id,omitempty"`
	Code         string `json:"code,omitempty"`
	AttachedRiskTriggerIDs
}

// CancelAllOrdersResult summarizes cancel-all.
//
// CancelledOrders lists the orders whose cancellation was requested, oldest
// first, at most 200; Truncated is true when TotalCancelled counts more. The
// order stream and order reads report every one. Both are empty for a dry run.
//
// ListingStatus is "included" when CancelledOrders is the listing, possibly
// empty. A retry with the same request ID within 120 seconds returns the
// original status and counts with an empty CancelledOrders and ListingStatus
// "not_retained_read_open_orders": the empty list does not mean nothing was
// canceled, and the retry requests no new cancellations. Read open orders for
// current state. An unknown value reads "unknown(<n>)".
type CancelAllOrdersResult struct {
	Status           string           `json:"status,omitempty"`
	MatchedOrders    int              `json:"matched_orders,omitempty"`
	SubmittedCancels int              `json:"submitted_cancels,omitempty"`
	FailedCancels    int              `json:"failed_cancels,omitempty"`
	CancelledOrders  []CancelledOrder `json:"cancelled_orders,omitempty"`
	TotalCancelled   int              `json:"total_cancelled,omitempty"`
	Truncated        bool             `json:"truncated,omitempty"`
	ListingStatus    string           `json:"listing_status,omitempty"`
}

// CancelledOrder is one order a cancel-all requested to cancel.
type CancelledOrder struct {
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id,omitempty"`
}

// BatchReplaceAdmissionItem is one admission outcome from batch_replace.
type BatchReplaceAdmissionItem struct {
	ItemIndex uint32 `json:"item_index"`
	Status    string `json:"status"`
	// OldOrderID and ReplacementOrderID are empty when the server sent none:
	// an amended or rejected item has no successor, and an item refused by
	// client order ID may name no order.
	OldOrderID         string `json:"old_order_id,omitempty"`
	ReplacementOrderID string `json:"replacement_order_id,omitempty"`
	ClientOrderID      string `json:"client_order_id,omitempty"`
	Code               string `json:"code,omitempty"`
	// ActionTaken is replaced when a successor order was admitted, with queue
	// priority reset, or amended when the order keeps OldOrderID with no
	// successor: amended in place with its queue priority kept, or, when the new
	// total quantity is at or below its filled quantity, ended at its filled
	// quantity. Keep tracking OldOrderID for an amended item. Empty for rejected
	// items.
	ActionTaken string `json:"action_taken,omitempty"`
	// RateLimit is set when ErrorDetail.rate_limit is present on rejection.
	RateLimit *RateLimitDetail `json:"rate_limit,omitempty"`
	// Violations names the request fields a rejection asks to fix, in the
	// order the server sent them; empty when it named none.
	Violations []OrderFieldViolation `json:"violations,omitempty"`
}

// BatchReplaceOrdersResult is the durable admission receipt from batch_replace.
type BatchReplaceOrdersResult struct {
	BatchRequestID string                      `json:"batch_request_id"`
	Status         string                      `json:"status"`
	Results        []BatchReplaceAdmissionItem `json:"results"`
	AcceptedCount  int                         `json:"accepted_count,omitempty"`
	RejectedCount  int                         `json:"rejected_count,omitempty"`
	AcceptedTsNs   uint64                      `json:"accepted_ts_ns,omitempty"`
}

// BatchReplaceStatusItem is one recoverable execution status from get_batch_replace_status.
//
// A rejected item changed nothing: its order keeps OldOrderID, its previous
// price and quantity, and no replacement order exists. This includes an item
// admitted and then refused when applied, for example a post-only order moved
// to a crossing price.
type BatchReplaceStatusItem struct {
	ItemIndex  uint32 `json:"item_index"`
	Phase      string `json:"phase"`
	OldOrderID string `json:"old_order_id,omitempty"`
	// ReplacementOrderID is the successor order, empty when none exists.
	ReplacementOrderID string `json:"replacement_order_id,omitempty"`
	OrderStatus        string `json:"order_status,omitempty"`
	// Code is the refusal or terminal code as sent, without the ERROR_CODE_
	// prefix, for example POST_ONLY_CROSS.
	Code string `json:"code,omitempty"`
	// ErrorCode is Code as the orders.v1 ErrorCode name a single ModifyOrder
	// returns, for example ERROR_CODE_POST_ONLY_CROSS. A code newer than this
	// SDK is kept as sent.
	ErrorCode   string `json:"error_code,omitempty"`
	UpdatedTsNs uint64 `json:"updated_ts_ns,omitempty"`
	// ActionTaken is replaced when a successor exists, or amended when the
	// order keeps OldOrderID with no successor, in place or ended at its filled
	// quantity; OrderStatus then describes OldOrderID. Empty for rejected items.
	ActionTaken string `json:"action_taken,omitempty"`
}

// BatchReplaceStatusResult is the durable batch status from get_batch_replace_status.
type BatchReplaceStatusResult struct {
	BatchRequestID  string                   `json:"batch_request_id"`
	AdmissionStatus string                   `json:"admission_status"`
	Items           []BatchReplaceStatusItem `json:"items"`
	AcceptedCount   int                      `json:"accepted_count,omitempty"`
	RejectedCount   int                      `json:"rejected_count,omitempty"`
	AcceptedTsNs    uint64                   `json:"accepted_ts_ns,omitempty"`
	UpdatedTsNs     uint64                   `json:"updated_ts_ns,omitempty"`
}

// BatchReplaceStatusSettled reports whether every replacement has reached a
// stable post-admission phase. "working" means live, not execution-final.
func (s BatchReplaceStatusResult) BatchReplaceStatusSettled() bool {
	return IsBatchReplaceSettled(s)
}

// BatchReplaceStatusSettled reports whether every batch-replace item has
// reached a stable post-admission phase.
func BatchReplaceStatusSettled(status BatchReplaceStatusResult) bool {
	return IsBatchReplaceSettled(status)
}

// IsBatchReplaceSettled reports whether all batch-replace items are working,
// rejected, or terminal. An empty status is not settled.
func IsBatchReplaceSettled(status BatchReplaceStatusResult) bool {
	if len(status.Items) == 0 {
		return false
	}
	for _, item := range status.Items {
		switch item.Phase {
		case "working", "rejected", "terminal":
		default:
			return false
		}
	}
	return true
}

// BatchCreateResultItem is one batch create outcome.
type BatchCreateResultItem struct {
	Status        string `json:"status"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	// ResolvedBaseQtyScaled, SubmittedMaxQuoteDebitScaled and
	// SubmittedQuoteProceedsScaled are an accepted item's echoes, as on
	// OrderMutationResult: the gross base quantity resolved for execution at
	// the pair's base_quantity_scale, and the quote budget of a BUY or the
	// quote proceeds target of a SELL sized by a quote amount, at the pair's
	// quote_quantity_scale. Empty when not sent.
	ResolvedBaseQtyScaled        string `json:"resolved_base_qty_scaled,omitempty"`
	SubmittedMaxQuoteDebitScaled string `json:"submitted_max_quote_debit_scaled,omitempty"`
	SubmittedQuoteProceedsScaled string `json:"submitted_quote_proceeds_scaled,omitempty"`
	// AttachedRiskTriggerIDs are an accepted item's attached legs' trigger
	// IDs, as on OrderMutationResult; empty for a leg the order does not have.
	AttachedRiskTriggerIDs
	Code string `json:"code,omitempty"`
	// ExistingOrderID is the order already holding ClientOrderID on an
	// ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID rejection, when known. It
	// is empty for duplicates rejected within the same batch.
	ExistingOrderID string `json:"existing_order_id,omitempty"`
	// RateLimit is set when ErrorDetail.rate_limit is present on rejection.
	RateLimit *RateLimitDetail `json:"rate_limit,omitempty"`
	// Violations names the request fields a rejection asks to fix, in the
	// order the server sent them; empty when it named none.
	Violations []OrderFieldViolation `json:"violations,omitempty"`
}

// BatchCreateOrdersResult summarizes batch create.
type BatchCreateOrdersResult struct {
	Results       []BatchCreateResultItem `json:"results"`
	AcceptedCount int                     `json:"accepted_count,omitempty"`
	RejectedCount int                     `json:"rejected_count,omitempty"`
}

// BatchCancelResultItem is one batch cancel outcome. OrderID is the order the
// item resolved to, empty when the server resolved none, such as an
// ORDER_UNKNOWN rejection by client order ID.
type BatchCancelResultItem struct {
	Status        string `json:"status"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Code          string `json:"code,omitempty"`
	// RateLimit is set when ErrorDetail.rate_limit is present on rejection.
	RateLimit *RateLimitDetail `json:"rate_limit,omitempty"`
	// Violations names the request fields a rejection asks to fix, in the
	// order the server sent them; empty when it named none.
	Violations []OrderFieldViolation `json:"violations,omitempty"`
}

// BatchCancelOrdersResult summarizes batch cancel.
type BatchCancelOrdersResult struct {
	Results       []BatchCancelResultItem `json:"results"`
	AcceptedCount int                     `json:"accepted_count,omitempty"`
	RejectedCount int                     `json:"rejected_count,omitempty"`
}

// CancelAllAfterResult is returned from cancel-all-after.
//
// EffectiveScope is the recorded scope of the armed switch; nil when the
// switch is disabled.
type CancelAllAfterResult struct {
	Status              string              `json:"status,omitempty"`
	EffectiveTimeoutSec int                 `json:"effective_timeout_sec,omitempty"`
	ExpiresAtTsNs       string              `json:"expires_at_ts_ns,omitempty"`
	EffectiveScope      *DeadManSwitchScope `json:"effective_scope,omitempty"`
}

// DeadManSwitchScope is the scope an armed dead-man switch cancels on
// expiry: the account's working orders, including working child orders.
// Waiting triggers stay active. SymbolID 0 means every symbol and an empty
// Side means both sides.
type DeadManSwitchScope struct {
	AccountID string `json:"account_id,omitempty"`
	SymbolID  uint32 `json:"symbol_id,omitempty"`
	Side      string `json:"side,omitempty"`
}

// AssetBalance is a ledger balance row.
type AssetBalance struct {
	AssetID   uint32 `json:"asset_id"`
	Trading   string `json:"trading,omitempty"`
	Funding   string `json:"funding,omitempty"`
	Reserved  string `json:"reserved,omitempty"`
	Available string `json:"available,omitempty"`
	// TradingRevision orders the atomic trading/reserved/available state.
	TradingRevision uint64 `json:"trading_revision,omitempty"`
	// FundingRevision orders funding state independently of trading.
	FundingRevision uint64 `json:"funding_revision,omitempty"`
}

// BalancesList holds balance rows.
type BalancesList struct {
	Balances []AssetBalance `json:"balances"`
}

// BalanceHistorySeries is one balance history series.
type BalanceHistorySeries struct {
	AssetID     uint32   `json:"asset_id"`
	AccountCode int32    `json:"account_code"`
	BalanceQ    []uint64 `json:"balance_q"`
}

// BalanceHistory is balance history response.
type BalanceHistory struct {
	Range      string                 `json:"range"`
	Bucket     string                 `json:"bucket,omitempty"`
	StartTsSec int64                  `json:"start_ts_sec,omitempty"`
	EndTsSec   int64                  `json:"end_ts_sec,omitempty"`
	Points     int                    `json:"points,omitempty"`
	Series     []BalanceHistorySeries `json:"series"`
}

// EquityHistorySeries is one equity history series.
type EquityHistorySeries struct {
	AccountCode        uint32  `json:"account_code,omitempty"`
	AccountName        string  `json:"account_name,omitempty"`
	AssetID            uint32  `json:"asset_id,omitempty"`
	AssetSymbol        string  `json:"asset_symbol,omitempty"`
	PortfolioAccountID string  `json:"portfolio_account_id,omitempty"`
	PortfolioRemaining bool    `json:"portfolio_remaining,omitempty"`
	EquityQ            []int64 `json:"equity_q"`
}

// EquityHistory is equity history response.
type EquityHistory struct {
	Range      string                `json:"range"`
	Bucket     string                `json:"bucket,omitempty"`
	StartTsSec int64                 `json:"start_ts_sec,omitempty"`
	EndTsSec   int64                 `json:"end_ts_sec,omitempty"`
	QuoteAsset string                `json:"quote_asset,omitempty"`
	Points     int                   `json:"points,omitempty"`
	Series     []EquityHistorySeries `json:"series"`
	// Raw BTC-USDT closes scaled by 1e9, aligned to the implicit timestamps.
	BTCPricesQ []int64 `json:"btc_prices_q"`
}

// TransferSide is one display side of a ledger transfer.
//
// Kind is a snake_case label (funding_account, trading_account,
// external_address, private_counterparty, fee_account, system_account).
// ChainID is the Zipper ChainConfig.chain_id for EXTERNAL_ADDRESS sides, not
// a network-native identifier such as an EIP-155 chain ID.
type TransferSide struct {
	Kind      string  `json:"kind,omitempty"`
	AccountID string  `json:"account_id,omitempty"`
	Address   string  `json:"address,omitempty"`
	ChainID   *uint32 `json:"chain_id,omitempty"`
}

// LedgerTransfer is a ledger transfer row.
type LedgerTransfer struct {
	// TransferID is the unsigned 128-bit decimal transfer identity shared by
	// both legs of one transfer. Empty when the server did not send one.
	TransferID   string `json:"transfer_id,omitempty"`
	AssetID      uint32 `json:"asset_id,omitempty"`
	Amount       string `json:"amount,omitempty"`
	TransferType int    `json:"transfer_type,omitempty"`
	AccountCode  int    `json:"account_code,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
	Pending      bool   `json:"pending,omitempty"`
	// TxID is the transfer's lifecycle flow ID (the contract's flow_id), the
	// ID Lifecycle.GetFlow accepts; empty when the transfer belongs to no
	// flow. A Trading-to-Funding transfer carries it from its first realtime
	// update. The WITHDRAW_FAILED (1075) debit of a failed withdrawal carries
	// its withdrawal's flow ID in Transfers.List, joined when history is read:
	// a read made before the withdrawal's outcome is recorded, and the
	// realtime transfer stream, leave it empty, so reload recent transfers
	// with Transfers.List to get the link.
	TxID        string        `json:"tx_id,omitempty"`
	IsDebit     bool          `json:"is_debit,omitempty"`
	Source      *TransferSide `json:"source,omitempty"`
	Destination *TransferSide `json:"destination,omitempty"`
	// BalanceAfter is the account bucket's balance after this transfer, an
	// exact decimal integer at the fixed 18-decimal ledger scale, like
	// Amount. Nil when the server sent none; a zero balance is "0".
	BalanceAfter *string `json:"balance_after,omitempty"`
	// LinkID is the decimal correlation ID that groups related transfer
	// legs. Empty when the server has none.
	LinkID string `json:"link_id,omitempty"`
}

// TransfersList holds ledger transfers.
type TransfersList struct {
	Transfers []LedgerTransfer `json:"transfers"`
	// Opaque continuation token; empty means the terminal page.
	NextPageToken string `json:"next_page_token"`
}

// InternalTransferResult is an internal transfer outcome.
type InternalTransferResult struct {
	RequestID  string            `json:"request_id,omitempty"`
	TransferID string            `json:"transfer_id,omitempty"`
	AssetID    uint32            `json:"asset_id,omitempty"`
	AssetCode  string            `json:"asset_code,omitempty"`
	Quantity   AssetAmountScaled `json:"quantity,omitempty"`
	// Status is how the service answered the request: "accepted", "rejected"
	// or "failed"; empty when the response names none, and "UNKNOWN(n)" for a
	// status this SDK version does not know. "accepted" means the transfer has
	// a durable transfer ID, not that it has settled.
	Status string `json:"status,omitempty"`
	// FlowID is the transfer's lifecycle flow, for Lifecycle.GetFlow; its
	// ledger settlement step names TransferID. The flow may become readable
	// after the receipt. Empty when the server sends none.
	FlowID string `json:"flow_id,omitempty"`
}

// DepositAddress is a chain deposit address.
type DepositAddress struct {
	ChainID        uint32 `json:"chain_id,omitempty"`
	DepositAddress string `json:"deposit_address,omitempty"`
}

// DepositAddressesList lists deposit addresses.
type DepositAddressesList struct {
	Addresses []DepositAddress `json:"addresses"`
}

// WithdrawIntentResult is a withdraw intent outcome.
type WithdrawIntentResult struct {
	IntentID string `json:"intent_id,omitempty"`
	Status   string `json:"status,omitempty"`
	FlowID   string `json:"flow_id,omitempty"`
}

// WithdrawDestinationValidation is the user-safe outcome of
// WithdrawService.ValidateDestination.
type WithdrawDestinationValidation struct {
	Valid                       bool   `json:"valid"`
	Code                        string `json:"code,omitempty"`
	Message                     string `json:"message,omitempty"`
	CanonicalDestinationAddress string `json:"canonical_destination_address,omitempty"`
}

// ZipperReasonDetails is the Zipper-specific failure detail on a lifecycle flow.
type ZipperReasonDetails struct {
	Code     int32  `json:"code,omitempty"`
	ReasonID string `json:"reason_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

// LifecycleFlowSummary is a lifecycle flow header.
//
// IsDelayed is true when an open flow has made no progress for well beyond
// the expected time of its current step: show it as delayed. The flow still
// completes normally once progress resumes. It is always false for terminal
// flows and for transaction lookups, which do not report it.
type LifecycleFlowSummary struct {
	IntentID            string               `json:"intent_id"`
	FlowKind            string               `json:"flow_kind,omitempty"`
	LatestStep          string               `json:"latest_step,omitempty"`
	IsOpen              bool                 `json:"is_open,omitempty"`
	IsTerminal          bool                 `json:"is_terminal,omitempty"`
	IsDelayed           bool                 `json:"is_delayed,omitempty"`
	OwnerAccountID      string               `json:"owner_account_id,omitempty"`
	SmartAccountAddress string               `json:"smart_account_address,omitempty"`
	LifecycleReason     string               `json:"lifecycle_reason,omitempty"`
	ZipperReason        *ZipperReasonDetails `json:"zipper_reason,omitempty"`
	// Participation is how the selected account takes part in the flow, on a
	// ListFlows row of a list that selects an account: "owner" (its deposit,
	// withdrawal or a transfer it sent, including a move between its own
	// balances) or "recipient" (a transfer another account sent to it). It is
	// "unspecified" on an unscoped list, a flow detail and a realtime update,
	// and empty on a transaction lookup.
	Participation string `json:"participation,omitempty"`
	// SourceAddress and DestinationAddress are the flow's source and
	// destination addresses. For a transfer, the counterparty is
	// DestinationAddress when the account sent it and SourceAddress (with
	// OwnerAccountID) when it received it.
	SourceAddress      string `json:"source_address,omitempty"`
	DestinationAddress string `json:"destination_address,omitempty"`
}

// LifecycleFlowsList lists lifecycle flows.
type LifecycleFlowsList struct {
	Flows         []LifecycleFlowSummary `json:"flows"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
}

// Trigger is a conditional order trigger.
//
// For a trailing stop, Side is the child's side: a sell trails the peak
// (Details.PeakPrice) and a buy trails the trough (Details.TroughPrice).
// LimitPrice is then a buy's maximum price, the most its child pays per
// unit; it is empty for a sell.
//
// MaxSlippage (ticks) or MaxSlippageBps is the slippage cap of a stop-loss or
// take-profit market child, of a trailing stop (also in Details), or of each
// slice of a market TWAP, in the variant the trigger was given; both are zero
// when no cap is set and the pair's default market slippage applies.
//
// Symbol is the pair name from the loaded catalogs; empty without them.
type Trigger struct {
	TriggerID               string          `json:"trigger_id,omitempty"`
	SubaccountID            string          `json:"subaccount_id,omitempty"`
	SymbolID                uint32          `json:"symbol_id,omitempty"`
	Symbol                  string          `json:"symbol,omitempty"`
	TriggerType             string          `json:"trigger_type,omitempty"`
	Status                  string          `json:"status,omitempty"`
	ParentOrderID           string          `json:"parent_order_id,omitempty"`
	Side                    string          `json:"side,omitempty"`
	OrderType               string          `json:"order_type,omitempty"`
	TimeInForce             string          `json:"time_in_force,omitempty"`
	Qty                     QtyScaled       `json:"qty,omitempty"`
	LimitPrice              PriceTicks      `json:"limit_price,omitempty"`
	FeeAsset                string          `json:"fee_asset,omitempty"`
	SelfTradePreventionMode string          `json:"self_trade_prevention_mode,omitempty"`
	PostOnly                bool            `json:"post_only,omitempty"`
	TriggerPrice            PriceTicks      `json:"trigger_price,omitempty"`
	MaxSlippage             PriceTicks      `json:"max_slippage,omitempty"`
	MaxSlippageBps          int32           `json:"max_slippage_bps,omitempty"`
	ClientTriggerID         string          `json:"client_trigger_id,omitempty"`
	CreatedAt               *time.Time      `json:"created_at,omitempty"`
	UpdatedAt               *time.Time      `json:"updated_at,omitempty"`
	ArmedAt                 *time.Time      `json:"armed_at,omitempty"`
	CompletedAt             *time.Time      `json:"completed_at,omitempty"`
	CancelReason            string          `json:"cancel_reason,omitempty"`
	FailureReason           string          `json:"failure_reason,omitempty"`
	Details                 *TriggerDetails `json:"details,omitempty"`
	// Version is 1 for the trigger's first state and higher for every later
	// state change. Keep the copy with the highest version across reads and
	// realtime updates. Zero from a server that predates trigger versions.
	Version uint32 `json:"version,omitempty"`
	// OrderListID is the OCO this take-profit or stop-loss belongs to and
	// ClientListID its client OCO ID: both triggers of an OCO created with
	// CreateOco carry them, and neither has a parent order. Empty for any
	// other trigger.
	OrderListID  string `json:"order_list_id,omitempty"`
	ClientListID string `json:"client_list_id,omitempty"`
}

// TriggerDetails is a discriminated trigger strategy payload.
// Case is one of: "stop", "trailing", "twap", "ladder".
type TriggerDetails struct {
	Case string `json:"case"`

	// stop; on trailing, the current threshold once armed (it moves with the
	// peak or trough)
	TriggerPrice       PriceTicks `json:"trigger_price,omitempty"`
	TriggerPriceSource string     `json:"trigger_price_source,omitempty"`
	TriggerDirection   string     `json:"trigger_direction,omitempty"`

	// trailing; TrailingDistance is the distance in quote units per base
	// unit, decoded from 1e-9 quote-unit ticks like every price.
	TrailingDistance    PriceTicks `json:"trailing_distance,omitempty"`
	TrailingDistanceBps int32      `json:"trailing_distance_bps,omitempty"`
	ActivationPrice     PriceTicks `json:"activation_price,omitempty"`
	PeakPrice           PriceTicks `json:"peak_price,omitempty"`
	TroughPrice         PriceTicks `json:"trough_price,omitempty"`
	MaxSlippage         PriceTicks `json:"max_slippage,omitempty"`
	MaxSlippageBps      int32      `json:"max_slippage_bps,omitempty"`

	// twap
	TwapDurationMs      int64     `json:"twap_duration_ms,omitempty"`
	TwapSliceIntervalMs int64     `json:"twap_slice_interval_ms,omitempty"`
	SliceIdx            int32     `json:"slice_idx,omitempty"`
	SliceCount          int32     `json:"slice_count,omitempty"`
	ExecutedQty         QtyScaled `json:"executed_qty,omitempty"`

	// ladder
	LadderPriceMin     PriceTicks `json:"ladder_price_min,omitempty"`
	LadderPriceMax     PriceTicks `json:"ladder_price_max,omitempty"`
	LadderLevels       int32      `json:"ladder_levels,omitempty"`
	LadderDistribution string     `json:"ladder_distribution,omitempty"`
	ExecutedLevels     int32      `json:"executed_levels,omitempty"`
}

// TriggersList lists triggers.
type TriggersList struct {
	Triggers      []Trigger `json:"triggers"`
	Total         int       `json:"total,omitempty"`
	NextPageToken string    `json:"next_page_token,omitempty"`
}

// TriggerMutationResult is a trigger mutation outcome.
type TriggerMutationResult struct {
	TriggerID string `json:"trigger_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

// CreateOcoResult is the admission of an OCO. OrderListID is the OCO's ID,
// which both legs carry as Trigger.OrderListID; TakeProfitTriggerID and
// StopLossTriggerID are the legs' trigger IDs. AcceptedTsNs is when
// admission completed, in nanoseconds since the Unix epoch; 0 when the server
// sent no time.
type CreateOcoResult struct {
	OrderListID         string `json:"order_list_id,omitempty"`
	ClientListID        string `json:"client_list_id,omitempty"`
	TakeProfitTriggerID string `json:"take_profit_trigger_id,omitempty"`
	StopLossTriggerID   string `json:"stop_loss_trigger_id,omitempty"`
	AcceptedTsNs        uint64 `json:"accepted_ts_ns,omitempty"`
}

// CancelOcoResult is a canceled OCO: Status is both legs' status after the
// cancel, "canceled".
type CancelOcoResult struct {
	OrderListID string `json:"order_list_id,omitempty"`
	Status      string `json:"status,omitempty"`
	// TsNs is when the cancel was answered, in nanoseconds since the Unix
	// epoch; 0 when the server sent no time.
	TsNs uint64 `json:"ts_ns,omitempty"`
}

// BatchCancelTriggersResultItem is one trigger's outcome in a batch cancel,
// in request order. Status is "accepted" (the trigger's events report when
// it is canceled) or "rejected", with Code saying why:
// TRIGGER_NOT_FOUND (no trigger of the account has this ID),
// TRIGGER_STATUS_INVALID (it has already ended; treat it as done), or
// RATE_LIMIT_EXCEEDED, PAIR_BACKLOGGED and TRIGGER_CONTROL_IN_PROGRESS
// (retry the item in a new request with a new request ID).
type BatchCancelTriggersResultItem struct {
	Status    string `json:"status"`
	TriggerID string `json:"trigger_id,omitempty"`
	Code      string `json:"code,omitempty"`
	// RateLimit is set when the rejection carries rate-limit guidance.
	RateLimit *RateLimitDetail `json:"rate_limit,omitempty"`
	// Violations names the request fields a rejection asks to fix, in the
	// order the server sent them; empty when it named none.
	Violations []OrderFieldViolation `json:"violations,omitempty"`
}

// BatchCancelTriggersResult summarizes a batch trigger cancel.
type BatchCancelTriggersResult struct {
	Results       []BatchCancelTriggersResultItem `json:"results"`
	AcceptedCount int                             `json:"accepted_count,omitempty"`
	RejectedCount int                             `json:"rejected_count,omitempty"`
	// TsNs is the server time of the answer, in nanoseconds since the Unix
	// epoch; 0 when the server sent no time.
	TsNs uint64 `json:"ts_ns,omitempty"`
}

// TriggerEvent is a trigger lifecycle event.
//
// EventType is fired, canceled, updated, failed, activated (a trailing stop
// started tracking), armed (an attached take-profit, stop-loss or trailing
// stop started protecting its parent's filled quantity, once) or fire_refused.
// A type newer than this SDK reads as unknown(<n>). A ladder has no event types
// of its own: read its Status, completed, canceled or failed.
//
// A ladder level the order book rejects when it arrives (for example a
// post-only level that would cross) is a "fire_refused" event that follows
// that level's "fired" event: ChildSeq names the level, FireRefusalReason
// says why ("post_only_cross") and FirePrice is the level's price, while the
// ladder keeps working its other levels. Do not count a level whose "fired"
// is followed by a "fire_refused" of the same ChildSeq as placed. The ladder
// fails ("failed") only when every level was rejected.
//
// EventType "fire_refused" records a fire whose order was refused while the
// trigger stayed live: the market's or system's state (for example
// FireRefusalReason "market_halted" or "post_only_cross"; it waits for the
// next price through its trigger price), a remaining position below the
// market's minimums ("min_notional", "min_quantity", "step_size"; it fires
// again once the position changes), or "rate_limited" / "account_repairing"
// (it fires again shortly), or "self_trade_prevented": self-trade prevention
// expired an attached take-profit, stop-loss or trailing stop order against
// another of the account's resting orders (it fires again at a new trade at
// or beyond its trigger price, or once that order is gone or no longer at the
// best price). It is not terminal: the trigger keeps its status, so read
// Trigger.Status rather than inferring it from the last event. ChildSeq is
// the refused attempt (a later fire takes a higher one). For
// "self_trade_prevented" that attempt placed an order, which ended canceled
// with CancelReason "stp_expire_taker" or "stp_expire_both"; for every other
// reason it placed none. CancelReason / FailureReason stay empty. A refusal
// that ends the trigger is a "failed" event with FailureReason instead.
// Event types and reasons newer than this SDK read as "unknown(<n>)".
//
// FirePrice is the price that fired the trigger, nil when the server sent
// none: a time-scheduled TWAP slice has no price that caused it to fire. Read
// the child order for its execution price.
type TriggerEvent struct {
	TriggerID     string      `json:"trigger_id,omitempty"`
	SubaccountID  string      `json:"subaccount_id,omitempty"`
	SymbolID      uint32      `json:"symbol_id,omitempty"`
	TriggerType   string      `json:"trigger_type,omitempty"`
	EventType     string      `json:"event_type,omitempty"`
	TsNs          string      `json:"ts_ns,omitempty"`
	ChildSeq      int32       `json:"child_seq,omitempty"`
	ChildOrderID  string      `json:"child_order_id,omitempty"`
	FirePrice     *PriceTicks `json:"fire_price,omitempty"`
	CancelReason  string      `json:"cancel_reason,omitempty"`
	FailureReason string      `json:"failure_reason,omitempty"`
	// FireRefusalReason is why a "fire_refused" event's order was refused,
	// with the FailureReason labels. Empty on every other event.
	FireRefusalReason string `json:"fire_refusal_reason,omitempty"`
}

// TriggerEventsList lists trigger events.
type TriggerEventsList struct {
	Events        []TriggerEvent `json:"events"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

// ApiKeySummary summarizes an API key.
type ApiKeySummary struct {
	KeyID string `json:"key_id,omitempty"`
	Label string `json:"label,omitempty"`
	// Icon and Color are the user-chosen icon or emoji and color token for
	// display, as sent (at most 32 characters each); empty when unset.
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	// CreatedByActor is the read-only label of who created the key, such as
	// a username or account label; empty when the server sends none.
	CreatedByActor string `json:"created_by_actor,omitempty"`
	Status         string `json:"status,omitempty"`
	// Purpose is "STANDARD" for the user's own keys and "ASSISTANT" for keys
	// the user created for the Polyester assistant; "PURPOSE_UNSPECIFIED" from
	// a server that predates the field.
	Purpose          string `json:"purpose,omitempty"`
	PublicKeyEd25519 string `json:"public_key_ed25519,omitempty"`
	// SubaccountID is the public ID of the sub-account the key is scoped to;
	// empty for a key of the main account.
	SubaccountID string `json:"subaccount_id,omitempty"`
	// PolicyID is the public ID of the API policy attached to the key, as
	// ApiPolicy.PolicyID; empty when none is attached, and then the key has
	// no permissions.
	PolicyID string `json:"policy_id,omitempty"`
	// IPWhitelist is the CIDR allowlist of the addresses the key may be used
	// from, as sent; nil means no IP restriction.
	IPWhitelist []string   `json:"ip_whitelist,omitempty"`
	Revision    uint64     `json:"revision,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	// ExpiresAt is the exact UTC time after which the key cannot
	// authenticate, whatever its status; nil when it does not expire.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ApiKeysList lists API keys.
type ApiKeysList struct {
	Keys []ApiKeySummary `json:"keys"`
}

// ResolvedAccount is a resolved account record.
type ResolvedAccount struct {
	AccountID           string `json:"account_id,omitempty"`
	Username            string `json:"username,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
}

// ResolvedAccountsList lists resolved accounts.
type ResolvedAccountsList struct {
	Accounts []ResolvedAccount `json:"accounts"`
}

// WalletBalances are the zTokens the account's smart account wallet holds on
// Polyester Chain, read live from the chain by Withdraw.GetWalletBalances. A
// withdrawal that Zipper rejects is refunded here, net of its kept network
// fee.
type WalletBalances struct {
	// SmartAccountAddress is the smart account wallet that holds them.
	SmartAccountAddress string          `json:"smart_account_address"`
	Balances            []WalletBalance `json:"balances"`
}

// WalletBalance is one nonzero zToken balance of the smart account wallet.
type WalletBalance struct {
	ZippedAssetID  uint32 `json:"zipped_asset_id"`
	UnifiedAssetID uint32 `json:"unified_asset_id"`
	// ChainID is the chain the zToken stands for.
	ChainID       uint32 `json:"chain_id"`
	ZTokenAddress string `json:"ztoken_address"`
	// AmountE18 is the raw u128 amount in 18-decimal units, as a decimal
	// string; Amount is the same amount in whole units of the asset.
	AmountE18 string `json:"amount_e18"`
	Amount    string `json:"amount"`
}
