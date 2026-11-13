package decode

import (
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// referenceID is the public ID of an account or entry a row refers to; ""
// for 0, the contract's "none" (a root scope's sub-account, an unsaved
// destination's entry).
func referenceID(id uint64) string {
	if id == 0 {
		return ""
	}
	return codecs.FormatID(id)
}

func accountScope(msg *authv1.AccountScopeRef) *models.AccountScope {
	if msg == nil {
		return nil
	}
	return &models.AccountScope{
		ScopeType:     msg.GetScopeType().String(),
		RootAccountID: referenceID(msg.GetRootAccountId()),
		SubAccountID:  referenceID(msg.GetSubaccountId()),
	}
}

func externalAddress(msg *authv1.ExternalWithdrawAddress) *models.ExternalWithdrawAddress {
	if msg == nil {
		return nil
	}
	return &models.ExternalWithdrawAddress{PolychainChainID: msg.GetPolychainChainId(), Address: msg.GetAddress()}
}

func internalAccount(msg *authv1.InternalTransferAccount) *models.InternalTransferAccount {
	if msg == nil {
		return nil
	}
	return &models.InternalTransferAccount{
		RootAccountID:       referenceID(msg.GetRootAccountId()),
		TargetAccountID:     referenceID(msg.GetTargetAccountId()),
		TargetScopeType:     msg.GetTargetScopeType().String(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		RootUsername:        msg.GetRootUsername(),
		SubAccountLabel:     msg.GetSubaccountLabel(),
	}
}

func tagIDs(ids []uint64) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, codecs.FormatUint64ID(id))
	}
	return out
}

func tagFromProto(msg *authv1.AddressBookTag) models.AddressBookTag {
	if msg == nil {
		return models.AddressBookTag{}
	}
	return models.AddressBookTag{
		TagID:     codecs.FormatUint64ID(msg.GetTagId()),
		Scope:     accountScope(msg.GetScope()),
		Name:      msg.GetName(),
		Color:     msg.GetColor(),
		CreatedAt: timestampTime(msg.GetCreatedAt()),
		UpdatedAt: timestampTime(msg.GetUpdatedAt()),
	}
}

func tagsFromProto(items []*authv1.AddressBookTag) []models.AddressBookTag {
	out := make([]models.AddressBookTag, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, tagFromProto(item))
	}
	return out
}

func entryFromProto(msg *authv1.AddressBookEntry) models.AddressBookEntry {
	if msg == nil {
		return models.AddressBookEntry{}
	}
	return models.AddressBookEntry{
		AddressBookEntryID: codecs.FormatUint64ID(msg.GetAddressBookEntryId()),
		Scope:              accountScope(msg.GetScope()),
		Label:              msg.GetLabel(),
		Kind:               msg.GetKind().String(),
		Note:               msg.GetNote(),
		CreatedAt:          timestampTime(msg.GetCreatedAt()),
		UpdatedAt:          timestampTime(msg.GetUpdatedAt()),
		External:           externalAddress(msg.GetExternal()),
		Internal:           internalAccount(msg.GetInternal()),
		Revision:           msg.GetRevision(),
		Tags:               tagsFromProto(msg.GetTags()),
	}
}

func requireEntry(operation string, entry *authv1.AddressBookEntry) (models.AddressBookEntry, error) {
	if entry == nil {
		return models.AddressBookEntry{}, &sdkerrors.ResponseContractError{Operation: operation, Msg: "missing entry"}
	}
	return entryFromProto(entry), nil
}

func requireTag(operation string, tag *authv1.AddressBookTag) (models.AddressBookTag, error) {
	if tag == nil {
		return models.AddressBookTag{}, &sdkerrors.ResponseContractError{Operation: operation, Msg: "missing tag"}
	}
	return tagFromProto(tag), nil
}

func EntryFromCreateProto(msg *authv1.CreateAddressBookEntryResponse) (models.AddressBookEntry, error) {
	if msg == nil {
		return models.AddressBookEntry{}, &sdkerrors.ResponseContractError{Operation: "CreateAddressBookEntry", Msg: "missing entry"}
	}
	return requireEntry("CreateAddressBookEntry", msg.GetEntry())
}

func EntryFromUpdateProto(msg *authv1.UpdateAddressBookEntryResponse) (models.AddressBookEntry, error) {
	if msg == nil {
		return models.AddressBookEntry{}, &sdkerrors.ResponseContractError{Operation: "UpdateAddressBookEntry", Msg: "missing entry"}
	}
	return requireEntry("UpdateAddressBookEntry", msg.GetEntry())
}

func TagFromCreateProto(msg *authv1.CreateAddressBookTagResponse) (models.AddressBookTag, error) {
	if msg == nil {
		return models.AddressBookTag{}, &sdkerrors.ResponseContractError{Operation: "CreateAddressBookTag", Msg: "missing tag"}
	}
	return requireTag("CreateAddressBookTag", msg.GetTag())
}

func TagFromUpdateProto(msg *authv1.UpdateAddressBookTagResponse) (models.AddressBookTag, error) {
	if msg == nil {
		return models.AddressBookTag{}, &sdkerrors.ResponseContractError{Operation: "UpdateAddressBookTag", Msg: "missing tag"}
	}
	return requireTag("UpdateAddressBookTag", msg.GetTag())
}

func ListEntriesFromProto(msg *authv1.ListAddressBookEntriesResponse) models.AddressBookEntriesList {
	out := make([]models.AddressBookEntry, 0, len(msg.GetEntries()))
	for _, e := range msg.GetEntries() {
		out = append(out, entryFromProto(e))
	}
	return models.AddressBookEntriesList{Entries: out, NextPageToken: msg.GetNextPageToken()}
}

func bookFromProto(msg *authv1.AddressBook) models.AddressBookSummary {
	return models.AddressBookSummary{
		Scope:               accountScope(msg.GetScope()),
		CallerRole:          subaccountRoleName(msg.GetCallerRole()),
		Label:               msg.GetLabel(),
		OwnerUsername:       msg.GetOwnerUsername(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
	}
}

func booksFromProto(items []*authv1.AddressBook) []models.AddressBookSummary {
	out := make([]models.AddressBookSummary, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, bookFromProto(item))
		}
	}
	return out
}

// ListBooksFromProto decodes the address books the caller can read.
func ListBooksFromProto(msg *authv1.ListAddressBooksResponse) models.AddressBooksList {
	return models.AddressBooksList{Books: booksFromProto(msg.GetBooks())}
}

func externalEntryView(msg *authv1.ExternalAddressBookEntry) models.ExternalAddressBookEntryView {
	return models.ExternalAddressBookEntryView{
		AddressBookEntryID: codecs.FormatUint64ID(msg.GetAddressBookEntryId()),
		Scope:              accountScope(msg.GetScope()),
		Label:              msg.GetLabel(),
		Note:               msg.GetNote(),
		TagIDs:             tagIDs(msg.GetTagIds()),
		WhitelistStatus:    msg.GetWhitelistStatus().String(),
		PolychainChainID:   msg.GetPolychainChainId(),
		Address:            msg.GetAddress(),
		CreatedAt:          timestampTime(msg.GetCreatedAt()),
		UpdatedAt:          timestampTime(msg.GetUpdatedAt()),
		Revision:           msg.GetRevision(),
	}
}

func internalEntryView(msg *authv1.InternalAddressBookEntry) models.InternalAddressBookEntryView {
	return models.InternalAddressBookEntryView{
		AddressBookEntryID:  codecs.FormatUint64ID(msg.GetAddressBookEntryId()),
		Scope:               accountScope(msg.GetScope()),
		Label:               msg.GetLabel(),
		Note:                msg.GetNote(),
		TagIDs:              tagIDs(msg.GetTagIds()),
		WhitelistStatus:     msg.GetWhitelistStatus().String(),
		RootAccountID:       referenceID(msg.GetRootAccountId()),
		TargetAccountID:     referenceID(msg.GetTargetAccountId()),
		TargetScopeType:     msg.GetTargetScopeType().String(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		RootUsername:        msg.GetRootUsername(),
		SubAccountLabel:     msg.GetSubaccountLabel(),
		CreatedAt:           timestampTime(msg.GetCreatedAt()),
		UpdatedAt:           timestampTime(msg.GetUpdatedAt()),
		Revision:            msg.GetRevision(),
	}
}

func entriesView(msg *authv1.AddressBookEntriesView) *models.AddressBookEntriesView {
	if msg == nil {
		return nil
	}
	out := &models.AddressBookEntriesView{}
	for _, item := range msg.GetExternal() {
		if item != nil {
			out.External = append(out.External, externalEntryView(item))
		}
	}
	for _, item := range msg.GetInternal() {
		if item != nil {
			out.Internal = append(out.Internal, internalEntryView(item))
		}
	}
	return out
}

func recentDestinations(msg *authv1.AddressBookRecentDestinationsView) *models.AddressBookRecentDestinationsView {
	if msg == nil {
		return nil
	}
	out := &models.AddressBookRecentDestinationsView{}
	for _, item := range msg.GetExternal() {
		if item == nil {
			continue
		}
		out.External = append(out.External, models.ExternalRecentDestination{
			Scope:              accountScope(item.GetScope()),
			LastDirection:      item.GetLastDirection().String(),
			Saved:              item.GetSaved(),
			AddressBookEntryID: referenceID(item.GetAddressBookEntryId()),
			UseCount:           item.GetUseCount(),
			LastSeenAt:         timestampTime(item.GetLastSeenAt()),
			PolychainChainID:   item.GetPolychainChainId(),
			Address:            item.GetAddress(),
		})
	}
	for _, item := range msg.GetInternal() {
		if item == nil {
			continue
		}
		out.Internal = append(out.Internal, models.InternalRecentDestination{
			Scope:               accountScope(item.GetScope()),
			LastDirection:       item.GetLastDirection().String(),
			Saved:               item.GetSaved(),
			AddressBookEntryID:  referenceID(item.GetAddressBookEntryId()),
			UseCount:            item.GetUseCount(),
			LastSeenAt:          timestampTime(item.GetLastSeenAt()),
			RootAccountID:       referenceID(item.GetRootAccountId()),
			TargetAccountID:     referenceID(item.GetTargetAccountId()),
			TargetScopeType:     item.GetTargetScopeType().String(),
			SmartAccountAddress: item.GetSmartAccountAddress(),
			RootUsername:        item.GetRootUsername(),
			SubAccountLabel:     item.GetSubaccountLabel(),
		})
	}
	return out
}

func tagSummaries(items []*authv1.AddressBookTagSummary) []models.AddressBookTagSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]models.AddressBookTagSummary, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, models.AddressBookTagSummary{TagID: codecs.FormatUint64ID(item.GetTagId()), Name: item.GetName(), Color: item.GetColor()})
		}
	}
	return out
}

func withdrawWhitelistView(msg *authv1.WithdrawWhitelistView) *models.WithdrawWhitelistView {
	if msg == nil {
		return nil
	}
	out := &models.WithdrawWhitelistView{
		Scope:                     accountScope(msg.GetScope()),
		ExternalWhitelistRequired: msg.GetExternalWhitelistRequired(),
		InternalWhitelistRequired: msg.GetInternalWhitelistRequired(),
	}
	for _, item := range msg.GetActiveEntries() {
		if item == nil {
			continue
		}
		out.ActiveEntries = append(out.ActiveEntries, models.MirroredWithdrawWhitelistEntry{
			CanonicalAddress: item.GetCanonicalAddress(),
			RawAddressHex:    item.GetRawAddressHex(),
			UpdatedAt:        timestampTime(item.GetUpdatedAt()),
			PolychainChainID: item.GetPolychainChainId(),
		})
	}
	return out
}

// AddressBookViewFromProto decodes the composed address book view. Its IDs
// are public IDs, as ListEntries returns them.
func AddressBookViewFromProto(msg *authv1.GetAddressBookViewResponse) models.AddressBookView {
	if msg == nil {
		return models.AddressBookView{}
	}
	out := models.AddressBookView{
		Entries:                     entriesView(msg.GetEntries()),
		RecentDestinations:          recentDestinations(msg.GetRecentDestinations()),
		Tags:                        tagSummaries(msg.GetTags()),
		WithdrawWhitelist:           withdrawWhitelistView(msg.GetWithdrawWhitelist()),
		RecentDestinationsTruncated: msg.GetRecentDestinationsTruncated(),
		ViewRevision:                msg.GetViewRevision(),
	}
	if len(msg.GetBooks()) > 0 {
		out.Books = booksFromProto(msg.GetBooks())
	}
	return out
}

// AddressBookInvalidationFromProto decodes an address-book invalidation event.
func AddressBookInvalidationFromProto(msg *authv1.AddressBookViewInvalidated) models.AddressBookViewInvalidation {
	if msg == nil {
		return models.AddressBookViewInvalidation{}
	}
	out := models.AddressBookViewInvalidation{Scope: accountScope(msg.GetScope())}
	if ts := msg.GetInvalidatedAt(); ts != nil {
		out.InvalidatedAt = ts.AsTime().UTC().Format(time.RFC3339Nano)
	}
	out.ViewRevision = msg.GetViewRevision()
	return out
}

// WithdrawWhitelistViewFromGetProto decodes the withdraw whitelist view; the
// zero value when the response carries none.
func WithdrawWhitelistViewFromGetProto(msg *authv1.GetWithdrawWhitelistViewResponse) models.WithdrawWhitelistView {
	if view := withdrawWhitelistView(msg.GetView()); view != nil {
		return *view
	}
	return models.WithdrawWhitelistView{}
}

func counterpartyFromProto(msg *authv1.TransferCounterparty) models.TransferCounterparty {
	return models.TransferCounterparty{
		CounterpartyID:     codecs.FormatUint64ID(msg.GetCounterpartyId()),
		Scope:              accountScope(msg.GetScope()),
		Direction:          msg.GetDirection().String(),
		Kind:               msg.GetKind().String(),
		Saved:              msg.GetSaved(),
		AddressBookEntryID: referenceID(msg.GetAddressBookEntryId()),
		UseCount:           msg.GetUseCount(),
		FirstSeenAt:        timestampTime(msg.GetFirstSeenAt()),
		LastSeenAt:         timestampTime(msg.GetLastSeenAt()),
		External:           externalAddress(msg.GetExternal()),
		Internal:           internalAccount(msg.GetInternal()),
	}
}

func destinationFromProto(msg *authv1.TransferDestination) models.AddressBookTransferDestination {
	out := models.AddressBookTransferDestination{
		Scope:              accountScope(msg.GetScope()),
		Kind:               msg.GetKind().String(),
		Saved:              msg.GetSaved(),
		Whitelisted:        msg.GetWhitelisted(),
		WhitelistStatus:    msg.GetWhitelistStatus().String(),
		External:           externalAddress(msg.GetExternal()),
		Internal:           internalAccount(msg.GetInternal()),
		WhitelistUpdatedAt: timestampTime(msg.GetWhitelistUpdatedAt()),
	}
	if entry := msg.GetAddressBookEntry(); entry != nil {
		decoded := entryFromProto(entry)
		out.AddressBookEntry = &decoded
	}
	return out
}

func whitelistEntryFromProto(msg *authv1.InternalTransferWhitelistEntry) models.InternalTransferWhitelistEntry {
	return models.InternalTransferWhitelistEntry{
		EntryID:             codecs.FormatUint64ID(msg.GetEntryId()),
		Scope:               accountScope(msg.GetScope()),
		RootAccountID:       referenceID(msg.GetRootAccountId()),
		TargetAccountID:     referenceID(msg.GetTargetAccountId()),
		TargetScopeType:     msg.GetTargetScopeType().String(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		RootUsername:        msg.GetRootUsername(),
		SubAccountLabel:     msg.GetSubaccountLabel(),
		CreatedAt:           timestampTime(msg.GetCreatedAt()),
		UpdatedAt:           timestampTime(msg.GetUpdatedAt()),
		ResolutionStatus:    msg.GetResolutionStatus().String(),
	}
}

// ListCounterpartiesFromProto decodes transfer counterparties.
func ListCounterpartiesFromProto(msg *authv1.ListTransferCounterpartiesResponse) models.TransferCounterpartiesList {
	out := make([]models.TransferCounterparty, 0, len(msg.GetCounterparties()))
	for _, item := range msg.GetCounterparties() {
		if item != nil {
			out = append(out, counterpartyFromProto(item))
		}
	}
	return models.TransferCounterpartiesList{Counterparties: out, Truncated: msg.GetTruncated()}
}

// ListDestinationsFromProto decodes transfer destinations.
func ListDestinationsFromProto(msg *authv1.ListTransferDestinationsResponse) models.AddressBookTransferDestinationsList {
	out := make([]models.AddressBookTransferDestination, 0, len(msg.GetDestinations()))
	for _, item := range msg.GetDestinations() {
		if item != nil {
			out = append(out, destinationFromProto(item))
		}
	}
	return models.AddressBookTransferDestinationsList{Destinations: out, NextPageToken: msg.GetNextPageToken()}
}

// ListInternalWhitelistFromProto decodes internal transfer whitelist entries.
func ListInternalWhitelistFromProto(msg *authv1.ListInternalTransferWhitelistEntriesResponse) models.InternalTransferWhitelistEntriesList {
	out := make([]models.InternalTransferWhitelistEntry, 0, len(msg.GetEntries()))
	for _, item := range msg.GetEntries() {
		if item != nil {
			out = append(out, whitelistEntryFromProto(item))
		}
	}
	return models.InternalTransferWhitelistEntriesList{Entries: out, NextPageToken: msg.GetNextPageToken()}
}
