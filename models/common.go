package models

// CreateOrderRequest is the SDK input for orders.create.
type CreateOrderRequest struct {
	Symbol       *string     `json:"symbol,omitempty"`
	SymbolID     *uint32     `json:"symbol_id,omitempty"`
	Side         string      `json:"side"`
	OrderType    string      `json:"order_type"`
	TIF          *string     `json:"tif,omitempty"`
	Qty          QtyInput    `json:"qty"`
	Price        *PriceInput `json:"price,omitempty"`
	SubAccountID *string     `json:"sub_account_id,omitempty"`
	// ClientOrderID is optional. Set a stable non-empty value when you may retry
	// after an ambiguous failure, and reuse that same id on retry/reconciliation.
	ClientOrderID *string        `json:"client_order_id,omitempty"`
	PostOnly      bool           `json:"post_only,omitempty"`
	ExpiresAt     *string        `json:"expires_at,omitempty"`
	AttachedRisk  map[string]any `json:"attached_risk,omitempty"`
	// MarketClientRefPrice is the client-supplied reference price for MARKET orders.
	MarketClientRefPrice *PriceInput `json:"market_client_ref_price,omitempty"`
}

// BatchModifyItem is one item in orders.batch_modify.
type BatchModifyItem struct {
	OrderID          *string     `json:"order_id,omitempty"`
	ClientOrderID    *string     `json:"client_order_id,omitempty"`
	NewPrice         *PriceInput `json:"new_price,omitempty"`
	NewQty           *QtyInput   `json:"new_qty,omitempty"`
	Behavior         *string     `json:"behavior,omitempty"`
	NewClientOrderID *string     `json:"new_client_order_id,omitempty"`
}

// BatchCancelItem is one item in orders.batch_cancel.
type BatchCancelItem struct {
	OrderID       *string `json:"order_id,omitempty"`
	ClientOrderID *string `json:"client_order_id,omitempty"`
	SymbolID      *uint32 `json:"symbol_id,omitempty"`
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
}

// ApiData is an escape hatch for responses not yet modeled.
type ApiData struct {
	Raw map[string]any `json:"raw"`
}

// SpotConfig holds the raw spot pair catalog payload.
type SpotConfig struct {
	Raw map[string]any `json:"raw"`
}
