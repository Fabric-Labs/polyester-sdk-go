package models

// SpotMarketRule is a spot market policy rule.
type SpotMarketRule struct {
	Symbol string `json:"symbol,omitempty"`
}

// PerpMarketRule is a perp market policy rule.
type PerpMarketRule struct {
	Symbol       string `json:"symbol,omitempty"`
	MaxLeverageX int    `json:"max_leverage_x,omitempty"`
}

// SubaccountPolicy is a subaccount policy record.
type SubaccountPolicy struct {
	PolicyID    string `json:"policy_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Revision    uint64 `json:"revision,omitempty"`
}

// SubaccountPoliciesList lists subaccount policies.
type SubaccountPoliciesList struct {
	Policies []SubaccountPolicy `json:"policies"`
}

// ApiPolicy is an API key policy record.
type ApiPolicy struct {
	PolicyID    string `json:"policy_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Revision    uint64 `json:"revision,omitempty"`
}

// ApiPoliciesList lists API policies.
type ApiPoliciesList struct {
	Policies []ApiPolicy `json:"policies"`
}
