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
	// EventType is the event's entity kind and action, such as
	// "session:failed" or "api_key:created".
	EventType string `json:"event_type,omitempty"`
	// TsMs is CreatedAt in milliseconds since the Unix epoch; 0 when the
	// event names no time.
	TsMs int64 `json:"ts_ms,omitempty"`
	// CreatedAt is when the event occurred, exactly as sent; nil when the
	// event names no time.
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// Source is the client channel the event came from, such as "web"; ""
	// when unspecified and "unknown(n)" for a channel this SDK does not
	// know.
	Source string `json:"source,omitempty"`
	// IP is the public client IP of the request that caused the event, and
	// UserAgent its user agent; each empty when not recorded.
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	// SubaccountID is the public ID of the sub-account the event belongs
	// to; empty for the main account's own events. Set by
	// SubAccounts.ListAccountActivity.
	SubaccountID string `json:"subaccount_id,omitempty"`
	// Actor is who performed the event; nil only when the server sends
	// none.
	Actor *SubAccountActivityActor `json:"actor,omitempty"`
	// Item is the item the event affected; nil for an event recorded before
	// items were.
	Item *ActivityItem `json:"item,omitempty"`
	// Details is what the event changed; nil when it records no change,
	// such as a deletion, was recorded before details were, or is of a kind
	// this SDK does not know.
	Details *ActivityDetails `json:"details,omitempty"`
}

// SubAccountActivityActor is who performed an activity event: a user, an
// API key, Polyester itself, or an unauthenticated request (such as a failed
// sign-in).
type SubAccountActivityActor struct {
	// Kind is "user", "api_key", "system" or "unauthenticated"; "" when
	// unspecified and "unknown(n)" for a kind this SDK does not know.
	Kind string `json:"kind,omitempty"`
	// AccountID is the public ID of the account that acted, for a user or
	// an API key: a sub-account member is named by their own account, and a
	// QuickSwap broker's key by the broker's account. Empty for the system
	// and an unauthenticated request.
	AccountID string `json:"account_id,omitempty"`
	// Username is that account's username, when it has one.
	Username string `json:"username,omitempty"`
	// APIKeyID is the API key the actor signed with, as ApiKeySummary.KeyID
	// names it ("ak_" and 32 hex digits); set for an API key actor only.
	APIKeyID string `json:"api_key_id,omitempty"`
	// APIKeyKind is that key's kind when the event was recorded: "standard",
	// "assistant", "admin", "strategy_bot" or "broker"; "" when not an API
	// key and "unknown(n)" for a kind this SDK does not know.
	APIKeyKind string `json:"api_key_kind,omitempty"`
	// APIKeyLabel is that key's label, when it has one.
	APIKeyLabel string `json:"api_key_label,omitempty"`
	// SystemReason is why Polyester acted, for the system actor only, such
	// as "assistant_preset_updated"; "unknown(n)" for a reason this SDK does
	// not know.
	SystemReason string `json:"system_reason,omitempty"`
}

// ListAccountActivityOptions selects the events of
// SubAccounts.ListAccountActivity. Every filter is optional: an omitted one
// matches everything the caller may read.
type ListAccountActivityOptions struct {
	// SubAccountIDs (public IDs, at most 100, no repeats) and
	// IncludeMainAccount select the accounts the feed covers:
	// IncludeMainAccount alone is the main account only, and neither covers
	// the whole account.
	SubAccountIDs      []string
	IncludeMainAccount bool
	// EntityKinds, EventActions and Sources keep events of these kinds,
	// actions and client channels, as the labels of EventType spell them
	// (for example "session", "api_key"; "created", "failed"; "web"), no
	// repeats.
	EntityKinds  []string
	EventActions []string
	Sources      []string
	// APIKeyID keeps the events performed with this API key, as
	// ApiKeySummary.KeyID names it ("ak_" and 32 lowercase hex digits).
	APIKeyID *string
	// StartTime (inclusive) and EndTime (exclusive) bound the event time;
	// nil for no bound. When both are set, EndTime must be after StartTime.
	StartTime *time.Time
	EndTime   *time.Time
	// Limit is the page size: 0 for the server's default of 50, at most 200.
	Limit int
	// PageToken continues from an earlier page's NextPageToken; reuse it only
	// with the same filters.
	PageToken string
}

// SubAccountActivityList lists activity events.
type SubAccountActivityList struct {
	Events        []SubAccountActivityEvent `json:"events"`
	NextPageToken string                    `json:"next_page_token,omitempty"`
}
