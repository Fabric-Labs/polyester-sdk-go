package models

import "time"

// PriceAlert is a durable price alert: it watches one market's price and
// fires once when the price crosses its level. Alerts belong to the main
// account. Price alerts are available on devnet; they are not yet routed on
// testnet.
type PriceAlert struct {
	// AlertID is the alert's public ID.
	AlertID  string `json:"alert_id"`
	SymbolID uint32 `json:"symbol_id"`
	// Symbol is the market's name from the loaded catalogs; empty when they
	// do not know it.
	Symbol string `json:"symbol,omitempty"`
	// Direction is "above" (fires at price >= Price) or "below" (fires at
	// price <= Price); both are inclusive.
	Direction string `json:"direction"`
	// PriceSource is the price the alert watches: "last" (last traded price)
	// or "index" (index price).
	PriceSource string `json:"price_source"`
	// Price is the alert's level.
	Price PriceTicks `json:"price"`
	// ReferencePrice is the market's price when the alert was created or its
	// level last changed. An alert whose condition already held then starts
	// unarmed and fires only after the price moves to the other side and
	// crosses back.
	ReferencePrice PriceTicks `json:"reference_price"`
	Armed          bool       `json:"armed"`
	// Status is "active", "triggered" or "cancelled" (the contract's
	// spelling).
	Status string `json:"status"`
	// Channels are where the alert is delivered: "in_app" and/or "telegram".
	Channels  []string   `json:"channels"`
	Revision  uint64     `json:"revision"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// TriggeredAt and TriggeredPrice are set once the alert fired.
	TriggeredAt    *time.Time  `json:"triggered_at,omitempty"`
	TriggeredPrice *PriceTicks `json:"triggered_price,omitempty"`
}

// PriceAlertsPage is one page of Alerts.List, newest first.
type PriceAlertsPage struct {
	Alerts []PriceAlert `json:"alerts"`
	// NextPageToken continues the list with the same filters; empty on the
	// last page.
	NextPageToken string `json:"next_page_token,omitempty"`
}

// UpdatePriceAlertResult is the answer of Alerts.Update.
type UpdatePriceAlertResult struct {
	Alert PriceAlert `json:"alert"`
	// Outcome is what the call did where it was applied: "applied" (it
	// changed the alert), "unchanged" (the alert already held the requested
	// values) or "replayed" (its idempotency key was already used for this
	// request: nothing was applied now, and Alert is the alert as it is now).
	// Tell a retry from a fresh change by Outcome, never by Revision.
	Outcome string `json:"outcome"`
}
