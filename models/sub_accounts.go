package models

// SubAccount is a subaccount summary.
type SubAccount struct {
	SubaccountID        string `json:"subaccount_id,omitempty"`
	Label               string `json:"label,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
	Status              string `json:"status,omitempty"`
}

// SubAccountsList lists subaccounts.
type SubAccountsList struct {
	Subaccounts []SubAccount `json:"subaccounts"`
}

// GetSubaccountResult is a detailed subaccount view.
type GetSubaccountResult struct {
	Subaccount *SubAccount `json:"subaccount,omitempty"`
}

// CreateSubaccountResult is create subaccount outcome.
type CreateSubaccountResult struct {
	SubaccountID        string `json:"subaccount_id,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
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
