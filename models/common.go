package models

// CreateOrderRequest is the SDK input for orders.create.
type CreateOrderRequest struct {
	Symbol        *string        `json:"symbol,omitempty"`
	SymbolID      *uint32        `json:"symbol_id,omitempty"`
	Side          string         `json:"side"`
	OrderType     string         `json:"order_type"`
	TIF           *string        `json:"tif,omitempty"`
	Qty           string         `json:"qty"`
	Price         *string        `json:"price,omitempty"`
	SubAccountID  *string        `json:"sub_account_id,omitempty"`
	ClientOrderID *string        `json:"client_order_id,omitempty"`
	PostOnly      bool           `json:"post_only,omitempty"`
	ExpiresAt     *string        `json:"expires_at,omitempty"`
	AttachedRisk  map[string]any `json:"attached_risk,omitempty"`
}

// BatchModifyItem is one item in orders.batch_modify.
type BatchModifyItem struct {
	OrderID          *string `json:"order_id,omitempty"`
	ClientOrderID    *string `json:"client_order_id,omitempty"`
	NewPrice         *string `json:"new_price,omitempty"`
	NewQty           *string `json:"new_qty,omitempty"`
	Behavior         *string `json:"behavior,omitempty"`
	NewClientOrderID *string `json:"new_client_order_id,omitempty"`
}

// BatchCancelItem is one item in orders.batch_cancel.
type BatchCancelItem struct {
	OrderID       *string `json:"order_id,omitempty"`
	ClientOrderID *string `json:"client_order_id,omitempty"`
	SymbolID      *uint32 `json:"symbol_id,omitempty"`
}

// OrderbookLevel is one orderbook price level.
type OrderbookLevel struct {
	Price        string  `json:"price,omitempty"`
	Qty          string  `json:"qty,omitempty"`
	PriceTicks   string  `json:"price_ticks,omitempty"`
	QtyScaled    string  `json:"qty_scaled,omitempty"`
	PriceDisplay *string `json:"price_display,omitempty"`
	QtyDisplay   *string `json:"qty_display,omitempty"`
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
