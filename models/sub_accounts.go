package models

import "time"

// SubAccount is a subaccount summary.
type SubAccount struct {
	SubaccountID string `json:"subaccount_id,omitempty"`
	// Role is the caller's role on the sub-account: viewer, trader,
	// leveraged_trader, treasury, admin or owner. Empty when the contract
	// leaves it unspecified, as it does when Removed is true.
	Role string `json:"role,omitempty"`
	// Removed is true on a real-time update telling the caller it no longer
	// has access to this sub-account: remove it from the caller's list. A
	// change of role or access does not change Revision or UpdatedAt, so
	// apply each update as it arrives. Reads list only accessible
	// sub-accounts and leave it false.
	Removed             bool       `json:"removed,omitempty"`
	Label               string     `json:"label,omitempty"`
	SmartAccountAddress string     `json:"smart_account_address,omitempty"`
	Status              string     `json:"status,omitempty"`
	Revision            uint64     `json:"revision,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

// SubAccountsList lists subaccounts.
type SubAccountsList struct {
	Subaccounts []SubAccount `json:"subaccounts"`
}

// GetSubaccountResult is a detailed subaccount view.
type GetSubaccountResult struct {
	Subaccount *SubAccount `json:"subaccount,omitempty"`
	// APIKeys are the API keys scoped to the sub-account, read as
	// APIKeys.List reads them. Nil unless SubAccounts.Get was asked for
	// them and the server sent some: it sends them to an owner or admin of
	// the sub-account only.
	APIKeys []ApiKeySummary `json:"api_keys,omitempty"`
	// Members are the sub-account's current members, read as
	// SubAccounts.ListMembers reads them. Nil unless SubAccounts.Get was
	// asked for them and the server sent some: it sends them to a current
	// member, and leaves them out for an API key without
	// READ_SUBACCOUNT_MEMBERS.
	Members []SubAccountMember `json:"members,omitempty"`
	// Invites are the invitations visible to the caller, read as
	// SubAccounts.ListInvites reads them and filtered by the invites
	// direction passed to SubAccounts.Get. Nil unless SubAccounts.Get was
	// asked for them and the server sent some: it leaves them out for an API
	// key without READ_SUBACCOUNT_INVITES.
	Invites []SubAccountInvite `json:"invites,omitempty"`
	// Policy is the sub-account policy attached to the sub-account, read as
	// Policies.SubscribeSubaccountPolicies reads it. Nil unless
	// SubAccounts.Get was asked for it, the caller is a current member and a
	// policy is attached.
	Policy *SubaccountPolicy `json:"policy,omitempty"`
	// Balances are the sub-account's current balances, in the shape of
	// Balances.List. Nil unless SubAccounts.Get was asked for them.
	Balances *BalancesList `json:"balances,omitempty"`
}

// CreateSubaccountResult is create subaccount outcome.
type CreateSubaccountResult struct {
	SubaccountID        string `json:"subaccount_id,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
	Revision            uint64 `json:"revision,omitempty"`
}

// SubAccountMember is a subaccount member.
type SubAccountMember struct {
	GranteeAccountID string `json:"grantee_account_id,omitempty"`
	Role             string `json:"role,omitempty"`
}

// SubAccountMembersList lists members.
type SubAccountMembersList struct {
	Members []SubAccountMember `json:"members"`
}

// SubAccountInvite is a subaccount invite.
type SubAccountInvite struct {
	InviteID         string `json:"invite_id,omitempty"`
	GranteeAccountID string `json:"grantee_account_id,omitempty"`
	Role             string `json:"role,omitempty"`
	Status           string `json:"status,omitempty"`
}

// SubAccountInvitesList lists invites.
type SubAccountInvitesList struct {
	Invites []SubAccountInvite `json:"invites"`
}

// SubAccountActivityEvent is one activity event.
type SubAccountActivityEvent struct {
	EventType string `json:"event_type,omitempty"`
	TsMs      int64  `json:"ts_ms,omitempty"`
}

// SubAccountActivityList lists activity events.
type SubAccountActivityList struct {
	Events        []SubAccountActivityEvent `json:"events"`
	NextPageToken string                    `json:"next_page_token,omitempty"`
}
