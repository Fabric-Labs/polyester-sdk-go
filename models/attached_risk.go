package models

// AttachedRiskInput is the write-side attached-risk policy for order create,
// modify, and batch replace. It arms after the parent order fills and protects
// the parent's filled position.
//
// Set TakeProfit and/or one stop leg (StopLoss or TrailingStop). OCO links the
// take-profit with the stop leg and requires both. On modify and batch replace
// the policy replaces the order's current attached risk.
type AttachedRiskInput struct {
	TakeProfit   *RiskLegInput      `json:"take_profit,omitempty"`
	StopLoss     *RiskLegInput      `json:"stop_loss,omitempty"`
	TrailingStop *TrailingStopInput `json:"trailing_stop,omitempty"`
	OCO          bool               `json:"oco,omitempty"`
}

// RiskLegInput is an attached take-profit or stop-loss leg. It evaluates the
// price named by TriggerPriceSource against TriggerPrice. The child is a
// market IOC when LimitPrice is nil, otherwise a resting GTC limit at
// LimitPrice.
//
// TriggerPriceSource is "last" (the last traded price) or "index" (the
// market's index price); empty watches the last traded price. On a market
// without an index price, an "index" leg is refused by the server.
type RiskLegInput struct {
	TriggerPrice       PriceInput  `json:"trigger_price"`
	LimitPrice         *PriceInput `json:"limit_price,omitempty"`
	TriggerPriceSource string      `json:"trigger_price_source,omitempty"`
}

// TrailingStopInput is an attached trailing stop. Set exactly one of
// DistanceTicks or DistanceBps, and at most one of MaxSlippageTicks or
// MaxSlippageBps. ActivationPrice, when set, delays trailing until the price
// is reached. TriggerPriceSource names the price it watches and trails, as
// on RiskLegInput.
type TrailingStopInput struct {
	DistanceTicks      *int64      `json:"distance_ticks,omitempty"`
	DistanceBps        *int32      `json:"distance_bps,omitempty"`
	MaxSlippageTicks   *int64      `json:"max_slippage_ticks,omitempty"`
	MaxSlippageBps     *int32      `json:"max_slippage_bps,omitempty"`
	ActivationPrice    *PriceInput `json:"activation_price,omitempty"`
	TriggerPriceSource string      `json:"trigger_price_source,omitempty"`
}
