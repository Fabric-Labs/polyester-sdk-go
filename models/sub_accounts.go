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
	// Icon and Color are the owner's display choices; empty when unset.
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	// OwnerUsername, OwnerAvatarURL and OwnerRootSmartAccountAddress identify
	// the owning root account; empty when the server sends none.
	OwnerUsername                string `json:"owner_username,omitempty"`
	OwnerAvatarURL               string `json:"owner_avatar_url,omitempty"`
	OwnerRootSmartAccountAddress string `json:"owner_root_smart_account_address,omitempty"`
	// SubaccountPolicyID is the public ID of the attached sub-account
	// policy; empty when none is attached.
	SubaccountPolicyID string `json:"subaccount_policy_id,omitempty"`
	// RequireMemberMFA is true when delegated members need an enrolled MFA
	// factor for protected sub-account actions.
	RequireMemberMFA bool `json:"require_member_mfa,omitempty"`
	// SmartAccountSaltNonce is the smart-account derivation salt nonce. Nil
	// unless the server sends it, which it does to the owner only.
	SmartAccountSaltNonce *uint32 `json:"smart_account_salt_nonce,omitempty"`
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
	// Username, SmartAccountAddress and AvatarURL describe the member's root
	// account; empty when the server sends none.
	Username            string `json:"username,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
	AvatarURL           string `json:"avatar_url,omitempty"`
	// MFAEnrolled is true when the member has at least one MFA factor
	// enrolled on their root account.
	MFAEnrolled bool `json:"mfa_enrolled,omitempty"`
}

// SubAccountMembersList lists members.
type SubAccountMembersList struct {
	Members []SubAccountMember `json:"members"`
}

// SubAccountInvite is a subaccount invite.
type SubAccountInvite struct {
	InviteID string `json:"invite_id,omitempty"`
	// SubaccountID is the public ID of the sub-account the invite is for.
	SubaccountID     string `json:"subaccount_id,omitempty"`
	GranteeAccountID string `json:"grantee_account_id,omitempty"`
	// InviterAccountID is the public ID of the inviting root account; empty
	// when the server sends none.
	InviterAccountID string `json:"inviter_account_id,omitempty"`
	Role             string `json:"role,omitempty"`
	Status           string `json:"status,omitempty"`
	// CreatedAt is when the invite was sent. RespondedAt is when it was
	// accepted, declined or cancelled, nil while it is pending.
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	// Display fields; empty when the server sends none.
	GranteeUsername                string `json:"grantee_username,omitempty"`
	InviterUsername                string `json:"inviter_username,omitempty"`
	SubaccountLabel                string `json:"subaccount_label,omitempty"`
	InviterRootSmartAccountAddress string `json:"inviter_root_smart_account_address,omitempty"`
	GranteeRootSmartAccountAddress string `json:"grantee_root_smart_account_address,omitempty"`
	// RequireMemberMFA is the sub-account's current member MFA requirement:
	// when true, accepting needs an enrolled MFA factor and a recently
	// MFA-elevated session.
	RequireMemberMFA bool `json:"require_member_mfa,omitempty"`
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
