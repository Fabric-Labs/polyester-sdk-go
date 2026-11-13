package models

import "time"

// AddressBookEntry is one address book entry. Exactly one of External and
// Internal names its destination, as Kind says.
type AddressBookEntry struct {
	AddressBookEntryID string `json:"address_book_entry_id,omitempty"`
	// Scope is the address book the entry belongs to; nil when not sent.
	Scope     *AccountScope `json:"scope,omitempty"`
	Label     string        `json:"label,omitempty"`
	Kind      string        `json:"kind,omitempty"`
	Note      string        `json:"note,omitempty"`
	CreatedAt *time.Time    `json:"created_at,omitempty"`
	UpdatedAt *time.Time    `json:"updated_at,omitempty"`
	// External is set for an EXTERNAL_CHAIN entry.
	External *ExternalWithdrawAddress `json:"external,omitempty"`
	// Internal is set for an INTERNAL_ACCOUNT entry.
	Internal *InternalTransferAccount `json:"internal,omitempty"`
	Revision uint64                   `json:"revision,omitempty"`
	Tags     []AddressBookTag         `json:"tags,omitempty"`
}

// AddressBookEntriesList lists entries.
type AddressBookEntriesList struct {
	Entries       []AddressBookEntry `json:"entries"`
	NextPageToken string             `json:"next_page_token,omitempty"`
}

// ExternalWithdrawAddress is an on-chain destination.
type ExternalWithdrawAddress struct {
	PolychainChainID uint32 `json:"polychain_chain_id,omitempty"`
	Address          string `json:"address,omitempty"`
}

// InternalTransferAccount is a destination account on the exchange. Account
// IDs are public IDs.
type InternalTransferAccount struct {
	RootAccountID   string `json:"root_account_id,omitempty"`
	TargetAccountID string `json:"target_account_id,omitempty"`
	// TargetScopeType is SCOPE_ROOT or SCOPE_SUBACCOUNT.
	TargetScopeType     string `json:"target_scope_type,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
	RootUsername        string `json:"root_username,omitempty"`
	SubAccountLabel     string `json:"sub_account_label,omitempty"`
}

// AddressBookTag is an address book tag.
type AddressBookTag struct {
	TagID     string        `json:"tag_id,omitempty"`
	Scope     *AccountScope `json:"scope,omitempty"`
	Name      string        `json:"name,omitempty"`
	Color     string        `json:"color,omitempty"`
	CreatedAt *time.Time    `json:"created_at,omitempty"`
	UpdatedAt *time.Time    `json:"updated_at,omitempty"`
}

// AddressBookTagSummary is a tag as the address book view lists it.
type AddressBookTagSummary struct {
	TagID string `json:"tag_id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// AddressBookTagInput is input for creating a tag inline with an entry.
type AddressBookTagInput struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// AddressBookSummary is one address book the caller can read.
type AddressBookSummary struct {
	Scope *AccountScope `json:"scope,omitempty"`
	// CallerRole is the caller's role on the book, spelled as
	// SubAccount.Role: viewer, trader, leveraged_trader, treasury, admin or
	// owner; "" when not sent.
	CallerRole          string `json:"caller_role,omitempty"`
	Label               string `json:"label,omitempty"`
	OwnerUsername       string `json:"owner_username,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
}

// AddressBooksList lists address books.
type AddressBooksList struct {
	Books []AddressBookSummary `json:"books"`
}

// AccountScope names an address book: the root account's own, or one of
// its sub-accounts'.
type AccountScope struct {
	// ScopeType is SCOPE_ROOT or SCOPE_SUBACCOUNT.
	ScopeType string `json:"scope_type,omitempty"`
	// RootAccountID is the root account's public ID.
	RootAccountID string `json:"root_account_id,omitempty"`
	// SubAccountID is the sub-account's public ID; empty for a root scope.
	SubAccountID string `json:"sub_account_id,omitempty"`
}

// AddressBookViewInvalidation signals that cached address book views are stale.
type AddressBookViewInvalidation struct {
	// Scope is the address book that changed; nil when the event names none.
	Scope         *AccountScope `json:"scope,omitempty"`
	InvalidatedAt string        `json:"invalidated_at,omitempty"`
	ViewRevision  uint64        `json:"view_revision,omitempty"`
}

// AddressBookView is a composed address book view. Every ID in it is a
// public ID, the same as ListEntries returns for the same entry.
type AddressBookView struct {
	Books              []AddressBookSummary               `json:"books,omitempty"`
	Entries            *AddressBookEntriesView            `json:"entries,omitempty"`
	RecentDestinations *AddressBookRecentDestinationsView `json:"recent_destinations,omitempty"`
	Tags               []AddressBookTagSummary            `json:"tags,omitempty"`
	WithdrawWhitelist  *WithdrawWhitelistView             `json:"withdraw_whitelist,omitempty"`
	// RecentDestinationsTruncated is true when more recent destinations
	// exist than the view lists.
	RecentDestinationsTruncated bool   `json:"recent_destinations_truncated,omitempty"`
	ViewRevision                uint64 `json:"view_revision,omitempty"`
}

// AddressBookEntriesView lists the view's saved entries by destination kind.
type AddressBookEntriesView struct {
	External []ExternalAddressBookEntryView `json:"external,omitempty"`
	Internal []InternalAddressBookEntryView `json:"internal,omitempty"`
}

// ExternalAddressBookEntryView is a saved on-chain destination in the view.
type ExternalAddressBookEntryView struct {
	AddressBookEntryID string        `json:"address_book_entry_id,omitempty"`
	Scope              *AccountScope `json:"scope,omitempty"`
	Label              string        `json:"label,omitempty"`
	Note               string        `json:"note,omitempty"`
	TagIDs             []string      `json:"tag_ids,omitempty"`
	WhitelistStatus    string        `json:"whitelist_status,omitempty"`
	PolychainChainID   uint32        `json:"polychain_chain_id,omitempty"`
	Address            string        `json:"address,omitempty"`
	CreatedAt          *time.Time    `json:"created_at,omitempty"`
	UpdatedAt          *time.Time    `json:"updated_at,omitempty"`
	// Revision is this entry's own revision, the expectedRevision of an
	// UpdateEntry; not the view's ViewRevision.
	Revision uint64 `json:"revision,omitempty"`
}

// InternalAddressBookEntryView is a saved exchange-account destination in
// the view.
type InternalAddressBookEntryView struct {
	AddressBookEntryID  string        `json:"address_book_entry_id,omitempty"`
	Scope               *AccountScope `json:"scope,omitempty"`
	Label               string        `json:"label,omitempty"`
	Note                string        `json:"note,omitempty"`
	TagIDs              []string      `json:"tag_ids,omitempty"`
	WhitelistStatus     string        `json:"whitelist_status,omitempty"`
	RootAccountID       string        `json:"root_account_id,omitempty"`
	TargetAccountID     string        `json:"target_account_id,omitempty"`
	TargetScopeType     string        `json:"target_scope_type,omitempty"`
	SmartAccountAddress string        `json:"smart_account_address,omitempty"`
	RootUsername        string        `json:"root_username,omitempty"`
	SubAccountLabel     string        `json:"sub_account_label,omitempty"`
	CreatedAt           *time.Time    `json:"created_at,omitempty"`
	UpdatedAt           *time.Time    `json:"updated_at,omitempty"`
	// Revision is this entry's own revision, the expectedRevision of an
	// UpdateEntry; not the view's ViewRevision.
	Revision uint64 `json:"revision,omitempty"`
}

// AddressBookRecentDestinationsView lists recently used destinations.
type AddressBookRecentDestinationsView struct {
	External []ExternalRecentDestination `json:"external,omitempty"`
	Internal []InternalRecentDestination `json:"internal,omitempty"`
}

// ExternalRecentDestination is a recently used on-chain destination.
type ExternalRecentDestination struct {
	Scope         *AccountScope `json:"scope,omitempty"`
	LastDirection string        `json:"last_direction,omitempty"`
	Saved         bool          `json:"saved,omitempty"`
	// AddressBookEntryID is the saved entry's ID; empty when not saved.
	AddressBookEntryID string     `json:"address_book_entry_id,omitempty"`
	UseCount           uint64     `json:"use_count,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	PolychainChainID   uint32     `json:"polychain_chain_id,omitempty"`
	Address            string     `json:"address,omitempty"`
}

// InternalRecentDestination is a recently used exchange-account destination.
type InternalRecentDestination struct {
	Scope         *AccountScope `json:"scope,omitempty"`
	LastDirection string        `json:"last_direction,omitempty"`
	Saved         bool          `json:"saved,omitempty"`
	// AddressBookEntryID is the saved entry's ID; empty when not saved.
	AddressBookEntryID  string     `json:"address_book_entry_id,omitempty"`
	UseCount            uint64     `json:"use_count,omitempty"`
	LastSeenAt          *time.Time `json:"last_seen_at,omitempty"`
	RootAccountID       string     `json:"root_account_id,omitempty"`
	TargetAccountID     string     `json:"target_account_id,omitempty"`
	TargetScopeType     string     `json:"target_scope_type,omitempty"`
	SmartAccountAddress string     `json:"smart_account_address,omitempty"`
	RootUsername        string     `json:"root_username,omitempty"`
	SubAccountLabel     string     `json:"sub_account_label,omitempty"`
}

// WithdrawWhitelistView is withdraw whitelist state.
type WithdrawWhitelistView struct {
	Scope                     *AccountScope                    `json:"scope,omitempty"`
	ExternalWhitelistRequired bool                             `json:"external_whitelist_required,omitempty"`
	InternalWhitelistRequired bool                             `json:"internal_whitelist_required,omitempty"`
	ActiveEntries             []MirroredWithdrawWhitelistEntry `json:"active_entries,omitempty"`
}

// MirroredWithdrawWhitelistEntry is an active on-chain whitelist entry.
type MirroredWithdrawWhitelistEntry struct {
	CanonicalAddress string     `json:"canonical_address,omitempty"`
	RawAddressHex    string     `json:"raw_address_hex,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	PolychainChainID uint32     `json:"polychain_chain_id,omitempty"`
}

// TransferCounterparty is an account or address the caller has transferred
// with.
type TransferCounterparty struct {
	CounterpartyID string        `json:"counterparty_id,omitempty"`
	Scope          *AccountScope `json:"scope,omitempty"`
	Direction      string        `json:"direction,omitempty"`
	Kind           string        `json:"kind,omitempty"`
	Saved          bool          `json:"saved,omitempty"`
	// AddressBookEntryID is the saved entry's ID; empty when not saved.
	AddressBookEntryID string                   `json:"address_book_entry_id,omitempty"`
	UseCount           uint64                   `json:"use_count,omitempty"`
	FirstSeenAt        *time.Time               `json:"first_seen_at,omitempty"`
	LastSeenAt         *time.Time               `json:"last_seen_at,omitempty"`
	External           *ExternalWithdrawAddress `json:"external,omitempty"`
	Internal           *InternalTransferAccount `json:"internal,omitempty"`
}

// TransferCounterpartiesList lists counterparties.
type TransferCounterpartiesList struct {
	Counterparties []TransferCounterparty `json:"counterparties"`
	Truncated      bool                   `json:"truncated,omitempty"`
}

// AddressBookTransferDestination is a destination the caller can transfer to.
type AddressBookTransferDestination struct {
	Scope           *AccountScope `json:"scope,omitempty"`
	Kind            string        `json:"kind,omitempty"`
	Saved           bool          `json:"saved,omitempty"`
	Whitelisted     bool          `json:"whitelisted,omitempty"`
	WhitelistStatus string        `json:"whitelist_status,omitempty"`
	// AddressBookEntry is the saved entry; nil when not saved.
	AddressBookEntry   *AddressBookEntry        `json:"address_book_entry,omitempty"`
	External           *ExternalWithdrawAddress `json:"external,omitempty"`
	Internal           *InternalTransferAccount `json:"internal,omitempty"`
	WhitelistUpdatedAt *time.Time               `json:"whitelist_updated_at,omitempty"`
}

// AddressBookTransferDestinationsList lists destinations.
type AddressBookTransferDestinationsList struct {
	Destinations  []AddressBookTransferDestination `json:"destinations"`
	NextPageToken string                           `json:"next_page_token,omitempty"`
}

// InternalTransferWhitelistEntry is an internal transfer whitelist entry.
type InternalTransferWhitelistEntry struct {
	EntryID             string        `json:"entry_id,omitempty"`
	Scope               *AccountScope `json:"scope,omitempty"`
	RootAccountID       string        `json:"root_account_id,omitempty"`
	TargetAccountID     string        `json:"target_account_id,omitempty"`
	TargetScopeType     string        `json:"target_scope_type,omitempty"`
	SmartAccountAddress string        `json:"smart_account_address,omitempty"`
	RootUsername        string        `json:"root_username,omitempty"`
	SubAccountLabel     string        `json:"sub_account_label,omitempty"`
	CreatedAt           *time.Time    `json:"created_at,omitempty"`
	UpdatedAt           *time.Time    `json:"updated_at,omitempty"`
	ResolutionStatus    string        `json:"resolution_status,omitempty"`
}

// InternalTransferWhitelistEntriesList lists whitelist entries.
type InternalTransferWhitelistEntriesList struct {
	Entries       []InternalTransferWhitelistEntry `json:"entries"`
	NextPageToken string                           `json:"next_page_token,omitempty"`
}
