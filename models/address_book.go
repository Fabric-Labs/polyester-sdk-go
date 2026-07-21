package models

// AddressBookEntry is one address book entry.
type AddressBookEntry struct {
	AddressBookEntryID string `json:"address_book_entry_id,omitempty"`
	Label              string `json:"label,omitempty"`
	Kind               string `json:"kind,omitempty"`
	Revision           uint64 `json:"revision,omitempty"`
}

// AddressBookEntriesList lists entries.
type AddressBookEntriesList struct {
	Entries       []AddressBookEntry `json:"entries"`
	NextPageToken string             `json:"next_page_token,omitempty"`
}

// AddressBookTag is an address book tag.
type AddressBookTag struct {
	TagID string `json:"tag_id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// AddressBookTagInput is input for creating a tag inline with an entry.
type AddressBookTagInput struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// AddressBooksList lists address books.
type AddressBooksList struct {
	Books []map[string]any `json:"books"`
}

// AddressBookViewInvalidation signals that cached address book views are stale.
type AddressBookViewInvalidation struct {
	Scope         string `json:"scope,omitempty"`
	InvalidatedAt string `json:"invalidated_at,omitempty"`
}

// AddressBookView is a composed address book view.
type AddressBookView struct {
	Raw map[string]any `json:"raw"`
}

// WithdrawWhitelistView is withdraw whitelist state.
type WithdrawWhitelistView struct {
	Raw map[string]any `json:"raw"`
}

// TransferCounterparty is a transfer counterparty row.
type TransferCounterparty struct {
	Raw map[string]any `json:"raw"`
}

// TransferCounterpartiesList lists counterparties.
type TransferCounterpartiesList struct {
	Counterparties []TransferCounterparty `json:"counterparties"`
	Truncated      bool                   `json:"truncated,omitempty"`
}

// AddressBookTransferDestination is a transfer destination row.
type AddressBookTransferDestination struct {
	Raw map[string]any `json:"raw"`
}

// AddressBookTransferDestinationsList lists destinations.
type AddressBookTransferDestinationsList struct {
	Destinations  []AddressBookTransferDestination `json:"destinations"`
	NextPageToken string                           `json:"next_page_token,omitempty"`
}

// InternalTransferWhitelistEntry is an internal whitelist row.
type InternalTransferWhitelistEntry struct {
	Raw map[string]any `json:"raw"`
}

// InternalTransferWhitelistEntriesList lists whitelist entries.
type InternalTransferWhitelistEntriesList struct {
	Entries       []InternalTransferWhitelistEntry `json:"entries"`
	NextPageToken string                           `json:"next_page_token,omitempty"`
}
