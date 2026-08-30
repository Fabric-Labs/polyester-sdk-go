package decode

import (
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func tagFromProto(msg *authv1.AddressBookTag) models.AddressBookTag {
	if msg == nil {
		return models.AddressBookTag{}
	}
	return models.AddressBookTag{
		TagID: codecs.FormatUint64ID(msg.GetTagId()),
		Name:  msg.GetName(),
		Color: msg.GetColor(),
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
		Label:              msg.GetLabel(),
		Kind:               msg.GetKind().String(),
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

func ListBooksFromProto(msg *authv1.ListAddressBooksResponse) models.AddressBooksList {
	books := make([]map[string]any, 0, len(msg.GetBooks()))
	for _, b := range msg.GetBooks() {
		if raw, err := wire.ProtoToMap(b); err == nil {
			books = append(books, raw)
		}
	}
	return models.AddressBooksList{Books: books}
}

func AddressBookViewFromProto(msg *authv1.GetAddressBookViewResponse) models.AddressBookView {
	if msg == nil {
		return models.AddressBookView{}
	}
	raw, err := wire.ProtoToMap(msg)
	if err != nil {
		return models.AddressBookView{}
	}
	return models.AddressBookView{ViewRevision: msg.GetViewRevision(), Raw: raw}
}

// AddressBookInvalidationFromProto decodes an address-book invalidation event.
func AddressBookInvalidationFromProto(msg *authv1.AddressBookViewInvalidated) models.AddressBookViewInvalidation {
	if msg == nil {
		return models.AddressBookViewInvalidation{}
	}
	out := models.AddressBookViewInvalidation{}
	if scope := msg.GetScope(); scope != nil && scope.GetRootAccountId() != 0 {
		out.Scope = codecs.FormatID(scope.GetRootAccountId())
	}
	if ts := msg.GetInvalidatedAt(); ts != nil {
		out.InvalidatedAt = ts.AsTime().UTC().Format(time.RFC3339Nano)
	}
	out.ViewRevision = msg.GetViewRevision()
	return out
}

func WithdrawWhitelistViewFromGetProto(msg *authv1.GetWithdrawWhitelistViewResponse) models.WithdrawWhitelistView {
	if view := msg.GetView(); view != nil {
		if raw, err := wire.ProtoToMap(view); err == nil {
			return models.WithdrawWhitelistView{Raw: raw}
		}
	}
	raw, err := wire.ProtoToMap(msg)
	if err != nil {
		return models.WithdrawWhitelistView{}
	}
	return models.WithdrawWhitelistView{Raw: raw}
}

func protoRowsToCounterparties(items []*authv1.TransferCounterparty) []models.TransferCounterparty {
	out := make([]models.TransferCounterparty, 0, len(items))
	for _, item := range items {
		if raw, err := wire.ProtoToMap(item); err == nil {
			out = append(out, models.TransferCounterparty{Raw: raw})
		}
	}
	return out
}

func protoRowsToDestinations(items []*authv1.TransferDestination) []models.AddressBookTransferDestination {
	out := make([]models.AddressBookTransferDestination, 0, len(items))
	for _, item := range items {
		if raw, err := wire.ProtoToMap(item); err == nil {
			out = append(out, models.AddressBookTransferDestination{Raw: raw})
		}
	}
	return out
}

func protoRowsToWhitelistEntries(items []*authv1.InternalTransferWhitelistEntry) []models.InternalTransferWhitelistEntry {
	out := make([]models.InternalTransferWhitelistEntry, 0, len(items))
	for _, item := range items {
		if raw, err := wire.ProtoToMap(item); err == nil {
			out = append(out, models.InternalTransferWhitelistEntry{Raw: raw})
		}
	}
	return out
}

// ListCounterpartiesFromProto decodes transfer counterparties.
func ListCounterpartiesFromProto(msg *authv1.ListTransferCounterpartiesResponse) models.TransferCounterpartiesList {
	return models.TransferCounterpartiesList{
		Counterparties: protoRowsToCounterparties(msg.GetCounterparties()),
		Truncated:      msg.GetTruncated(),
	}
}

// ListDestinationsFromProto decodes transfer destinations.
func ListDestinationsFromProto(msg *authv1.ListTransferDestinationsResponse) models.AddressBookTransferDestinationsList {
	return models.AddressBookTransferDestinationsList{
		Destinations:  protoRowsToDestinations(msg.GetDestinations()),
		NextPageToken: msg.GetNextPageToken(),
	}
}

// ListInternalWhitelistFromProto decodes internal transfer whitelist entries.
func ListInternalWhitelistFromProto(msg *authv1.ListInternalTransferWhitelistEntriesResponse) models.InternalTransferWhitelistEntriesList {
	return models.InternalTransferWhitelistEntriesList{
		Entries:       protoRowsToWhitelistEntries(msg.GetEntries()),
		NextPageToken: msg.GetNextPageToken(),
	}
}
