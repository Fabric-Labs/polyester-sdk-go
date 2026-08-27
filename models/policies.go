package models

import "time"

// SpotMarketRule is a spot market policy rule.
type SpotMarketRule struct {
	SymbolID uint32 `json:"symbol_id,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
}

// SubaccountPolicy is a subaccount policy record.
type SubaccountPolicy struct {
	PolicyID         string           `json:"policy_id,omitempty"`
	Name             string           `json:"name,omitempty"`
	Description      string           `json:"description,omitempty"`
	Revision         uint64           `json:"revision,omitempty"`
	SpotMarkets      []SpotMarketRule `json:"spot_markets,omitempty"`
	SpotMarketScope  string           `json:"spot_market_scope,omitempty"`
	Actions          []string         `json:"actions,omitempty"`
	IsTemplate       bool             `json:"is_template,omitempty"`
	SourceTemplateID string           `json:"source_template_id,omitempty"`
	MaxOrderNotional uint64           `json:"max_order_notional,omitempty"`
	MaxOpenOrders    uint32           `json:"max_open_orders,omitempty"`
	TradingHalted    bool             `json:"trading_halted,omitempty"`
	Locked           bool             `json:"locked,omitempty"`
	ReviewAt         *time.Time       `json:"review_at,omitempty"`
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
	CreatedAt        *time.Time       `json:"created_at,omitempty"`
	UpdatedAt        *time.Time       `json:"updated_at,omitempty"`
}

// SubaccountPoliciesList lists subaccount policies.
type SubaccountPoliciesList struct {
	Policies []SubaccountPolicy `json:"policies"`
}

// ApiPolicy is an API key policy record.
type ApiPolicy struct {
	PolicyID         string           `json:"policy_id,omitempty"`
	Name             string           `json:"name,omitempty"`
	Description      string           `json:"description,omitempty"`
	Revision         uint64           `json:"revision,omitempty"`
	SpotMarkets      []SpotMarketRule `json:"spot_markets,omitempty"`
	SpotMarketScope  string           `json:"spot_market_scope,omitempty"`
	Actions          []string         `json:"actions,omitempty"`
	IsTemplate       bool             `json:"is_template,omitempty"`
	SourceTemplateID string           `json:"source_template_id,omitempty"`
	CreatedAt        *time.Time       `json:"created_at,omitempty"`
	UpdatedAt        *time.Time       `json:"updated_at,omitempty"`
}

// ApiPoliciesList lists API policies.
type ApiPoliciesList struct {
	Policies []ApiPolicy `json:"policies"`
}
