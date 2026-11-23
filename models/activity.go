package models

import (
	"time"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// ActivityItemKind names which ID an ActivityItem has, as the contract's
// ActivityItem field that is set: it says what the item is.
type ActivityItemKind string

// The kinds of item an activity event can affect. ActivityItemAPIKey and
// ActivityItemMFAFactor carry the string IDs those APIs use; every other
// kind carries a public ID.
const (
	// ActivityItemAccount is a main account, or the member account of a
	// sub-account membership.
	ActivityItemAccount          ActivityItemKind = "account_id"
	ActivityItemSubaccount       ActivityItemKind = "subaccount_id"
	ActivityItemInvite           ActivityItemKind = "invite_id"
	ActivityItemSubaccountPolicy ActivityItemKind = "subaccount_policy_id"
	ActivityItemAPIPolicy        ActivityItemKind = "api_policy_id"
	// ActivityItemAPIKey is an API key, as ApiKeySummary.KeyID names it.
	ActivityItemAPIKey ActivityItemKind = "api_key_id"
	// ActivityItemMFAFactor is an MFA factor, by its factor ID.
	ActivityItemMFAFactor        ActivityItemKind = "mfa_factor_id"
	ActivityItemAddressBookEntry ActivityItemKind = "address_book_entry_id"
	// ActivityItemBrokerAllowlistChange is a QuickSwap broker allowlist
	// change request.
	ActivityItemBrokerAllowlistChange ActivityItemKind = "broker_allowlist_change_id"
	ActivityItemConnectedApp          ActivityItemKind = "connected_app_id"
)

// ActivityItem is the item an activity event affected.
type ActivityItem struct {
	// Kind says what the item is; "" for an item with no ID: a sign-in
	// session, a failed sign-in or the recovery codes.
	Kind ActivityItemKind `json:"kind,omitempty"`
	// ID is the item's ID: a public ID, or for an API key or MFA factor the
	// string ID those APIs use. "" when Kind is "".
	ID string `json:"id,omitempty"`
	// Name is the item's display name when the event was recorded: a policy
	// name, an API key or address-book label, a sub-account label, a
	// member's username, an MFA factor label, a connected app's name or a
	// session's device.
	Name string `json:"name,omitempty"`
}

// ActivityChange is a value before (From) and after (To) an activity event.
// What an unset side reads as is documented on each field that holds one.
type ActivityChange[T any] struct {
	From T `json:"from"`
	To   T `json:"to"`
}

// ActivityListChange is what an activity event added to and removed from a
// list.
type ActivityListChange[T any] struct {
	Added   []T `json:"added,omitempty"`
	Removed []T `json:"removed,omitempty"`
}

// ActivityDetails is what an activity event changed. Exactly one field is
// set, for the kind of item the event affected.
//
// Within each kind, a nil change is one the event did not record. A text
// change reads an unset side as "", an ID change as "" and a time change as
// nil; times keep the contract's full nanosecond precision, and IDs are
// public IDs. Enums keep the contract's own types: an unset value reads as
// its ..._UNSPECIFIED constant (0), and one newer than this SDK as its
// number.
type ActivityDetails struct {
	Account *ActivityAccountDetails `json:"account,omitempty"`
	// Session is a sign-in, a failed sign-in, a connected app or an
	// MFA-elevated action.
	Session *ActivitySessionDetails `json:"session,omitempty"`
	APIKey  *ActivityAPIKeyDetails  `json:"api_key,omitempty"`
	// Policy is a sub-account or API key policy change.
	Policy     *ActivityPolicyDetails     `json:"policy,omitempty"`
	Subaccount *ActivitySubaccountDetails `json:"subaccount,omitempty"`
	// Member is a sub-account or QuickSwap broker membership change.
	Member *ActivityMemberDetails `json:"member,omitempty"`
	// Invite is a sub-account or QuickSwap broker invitation.
	Invite *ActivityInviteDetails `json:"invite,omitempty"`
	// Destination is an address-book change.
	Destination *ActivityDestinationDetails `json:"destination,omitempty"`
	// BrokerAllowlist is a QuickSwap broker allowlist change; the change
	// request is the event's item.
	BrokerAllowlist *ActivityBrokerAllowlistDetails `json:"broker_allowlist,omitempty"`
	// Security is an MFA or recovery-code change.
	Security *ActivitySecurityDetails `json:"security,omitempty"`
}

// ActivityAccountDetails is what an account event changed.
type ActivityAccountDetails struct {
	Username *ActivityChange[string] `json:"username,omitempty"`
	// GeneratedUsername is true when Polyester generated the new username
	// rather than the user choosing it.
	GeneratedUsername bool `json:"generated_username,omitempty"`
}

// ActivitySessionDetails is what a session event recorded.
type ActivitySessionDetails struct {
	// Method is how the user signed in, for a sign-in or a failed one.
	Method authv1.ActivitySignInMethod `json:"method,omitempty"`
	// WalletAddress is the wallet address that signed in, lowercase.
	WalletAddress string `json:"wallet_address,omitempty"`
	// Failure is why the sign-in failed, for a failed one.
	Failure authv1.ActivitySignInFailure `json:"failure,omitempty"`
	// Scopes are the scopes the user granted a connected app.
	Scopes []string `json:"scopes,omitempty"`
	// ElevatedAction is the action the session's recent MFA check was used
	// for.
	ElevatedAction authv1.ActivityElevatedAction `json:"elevated_action,omitempty"`
}

// ActivityAPIKeyDetails is what an API key event changed.
type ActivityAPIKeyDetails struct {
	Label   *ActivityChange[string]               `json:"label,omitempty"`
	Status  *ActivityChange[authv1.ApiKeyStatus]  `json:"status,omitempty"`
	Purpose *ActivityChange[authv1.ApiKeyPurpose] `json:"purpose,omitempty"`
	Icon    *ActivityChange[string]               `json:"icon,omitempty"`
	Color   *ActivityChange[string]               `json:"color,omitempty"`
	// IPAllowlist is the IP addresses or ranges added to and removed from
	// the key's allowlist, masked like the event's IP for callers who may
	// not see IP addresses.
	IPAllowlist *ActivityListChange[string] `json:"ip_allowlist,omitempty"`
	// ExpiresAt is the key's expiry.
	ExpiresAt *ActivityChange[*time.Time] `json:"expires_at,omitempty"`
	// APIPolicyID is the API key policy bound to the key, as public IDs.
	APIPolicyID *ActivityChange[string] `json:"api_policy_id,omitempty"`
	// StrategyBotID is the public ID of the strategy bot the key belongs
	// to, for a strategy bot key; "" otherwise.
	StrategyBotID string `json:"strategy_bot_id,omitempty"`
}

// ActivityPolicyDetails is what a sub-account or API key policy event
// changed.
type ActivityPolicyDetails struct {
	Name        *ActivityChange[string] `json:"name,omitempty"`
	Description *ActivityChange[string] `json:"description,omitempty"`
	// Actions is the actions the policy started and stopped allowing.
	Actions *ActivityListChange[authv1.PolicyAction] `json:"actions,omitempty"`
	// AllSpotMarkets is whether the policy allows every spot market.
	AllSpotMarkets *ActivityChange[bool] `json:"all_spot_markets,omitempty"`
	// MaxOrderNotional is the maximum notional of one order, in canonical
	// quote microunits (one unit is 0.000001 USDT); 0 is no cap.
	MaxOrderNotional *ActivityChange[uint64] `json:"max_order_notional,omitempty"`
	// MaxOpenOrders is the maximum number of resting orders; 0 is no cap.
	MaxOpenOrders *ActivityChange[uint64] `json:"max_open_orders,omitempty"`
	TradingHalted *ActivityChange[bool]   `json:"trading_halted,omitempty"`
	Locked        *ActivityChange[bool]   `json:"locked,omitempty"`
	// ReviewAt is the policy's review time.
	ReviewAt *ActivityChange[*time.Time] `json:"review_at,omitempty"`
	// ExpiresAt is the policy's expiry.
	ExpiresAt *ActivityChange[*time.Time] `json:"expires_at,omitempty"`
	// AssistantPreset is the AI assistant key preset the policy follows.
	AssistantPreset authv1.AssistantKeyPreset `json:"assistant_preset,omitempty"`
	// SourceTemplateID is the public ID of the template the policy was
	// copied from, and SourceTemplateRevision that template's revision when
	// it was copied; "" and 0 when none.
	SourceTemplateID       string `json:"source_template_id,omitempty"`
	SourceTemplateRevision uint64 `json:"source_template_revision,omitempty"`
	// SubaccountPolicyID is the sub-account policy bound to the event's
	// sub-account, as public IDs.
	SubaccountPolicyID *ActivityChange[string] `json:"subaccount_policy_id,omitempty"`
	// StrategyBotID is the public ID of the strategy bot the policy was
	// made for; "" otherwise.
	StrategyBotID string `json:"strategy_bot_id,omitempty"`
}

// ActivitySubaccountDetails is what a sub-account event changed.
type ActivitySubaccountDetails struct {
	Label  *ActivityChange[string]                  `json:"label,omitempty"`
	Status *ActivityChange[authv1.SubaccountStatus] `json:"status,omitempty"`
	Icon   *ActivityChange[string]                  `json:"icon,omitempty"`
	Color  *ActivityChange[string]                  `json:"color,omitempty"`
	// RequireMemberMFA is whether the sub-account's members must use MFA.
	RequireMemberMFA *ActivityChange[bool] `json:"require_member_mfa,omitempty"`
}

// ActivityMemberDetails is what a membership event changed.
type ActivityMemberDetails struct {
	// Role is the member's sub-account role; SUBACCOUNT_ROLE_UNSPECIFIED on
	// a side where the member had none.
	Role *ActivityChange[authv1.SubaccountRole] `json:"role,omitempty"`
	// BrokerRole is the member's QuickSwap broker role;
	// ACTIVITY_BROKER_ROLE_UNSPECIFIED on a side where the member had none.
	BrokerRole *ActivityChange[authv1.ActivityBrokerRole] `json:"broker_role,omitempty"`
	// OwnershipTransfer is true when the event moved a QuickSwap broker's
	// ownership to the member.
	OwnershipTransfer bool `json:"ownership_transfer,omitempty"`
}

// ActivityInviteDetails is what an invitation event recorded.
type ActivityInviteDetails struct {
	// GranteeAccountID is the public ID of the account invited.
	GranteeAccountID string `json:"grantee_account_id,omitempty"`
	// Role is the sub-account role offered.
	Role authv1.SubaccountRole `json:"role,omitempty"`
	// BrokerRole is the QuickSwap broker role offered.
	BrokerRole authv1.ActivityBrokerRole                      `json:"broker_role,omitempty"`
	Status     *ActivityChange[authv1.SubaccountInviteStatus] `json:"status,omitempty"`
}

// ActivityDestinationDetails is what an address-book event changed.
type ActivityDestinationDetails struct {
	Label *ActivityChange[string] `json:"label,omitempty"`
	// NoteChanged is true when the entry's note changed. The note itself is
	// not shown.
	NoteChanged bool `json:"note_changed,omitempty"`
	// TagIDs is the tags added to and removed from the entry, as public IDs.
	TagIDs *ActivityListChange[string] `json:"tag_ids,omitempty"`
}

// ActivityBrokerAllowlistDetails is what a QuickSwap broker allowlist event
// recorded.
type ActivityBrokerAllowlistDetails struct {
	// Action is whether the change adds or removes the destination.
	Action authv1.ActivityAllowlistAction `json:"action,omitempty"`
	// Destination is the destination address.
	Destination        string `json:"destination,omitempty"`
	DestinationChainID uint64 `json:"destination_chain_id,omitempty"`
	Label              string `json:"label,omitempty"`
}

// ActivitySecurityDetails is what an account security event changed.
type ActivitySecurityDetails struct {
	// FactorType is the type of the MFA factor added, changed, removed or
	// used.
	FactorType authv1.MFAFactorType    `json:"factor_type,omitempty"`
	Label      *ActivityChange[string] `json:"label,omitempty"`
	// ChallengePurpose is what a passed MFA check was for.
	ChallengePurpose authv1.MFAChallengePurpose `json:"challenge_purpose,omitempty"`
	// RecoveryCodesRotated is true when the recovery codes were replaced by
	// new ones.
	RecoveryCodesRotated bool `json:"recovery_codes_rotated,omitempty"`
}
