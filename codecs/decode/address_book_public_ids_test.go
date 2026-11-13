package decode

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	abEntryID   uint64 = 4611686018427387911
	abRootID    uint64 = 12297829382473034410
	abTargetID  uint64 = 281474976710677
	abSubID     uint64 = 9223372036854775837
	abTagID     uint64 = 7340032
	abSmartAddr        = "0x00000000000000000000000000000000000000a1"
)

var abCreated = time.Date(2026, 3, 4, 5, 6, 7, 825341000, time.UTC)

func abScope() *authv1.AccountScopeRef {
	return &authv1.AccountScopeRef{ScopeType: authv1.AccountScopeType_SCOPE_SUBACCOUNT, RootAccountId: abRootID, SubaccountId: abSubID}
}

func abInternalEntry() *authv1.AddressBookEntry {
	return &authv1.AddressBookEntry{
		AddressBookEntryId: abEntryID,
		Scope:              abScope(),
		Kind:               authv1.AddressBookEntryKind_INTERNAL_ACCOUNT,
		Label:              "treasury",
		Note:               "desk wallet",
		CreatedAt:          timestamppb.New(abCreated),
		UpdatedAt:          timestamppb.New(abCreated.Add(time.Second)),
		Entry: &authv1.AddressBookEntry_Internal{Internal: &authv1.InternalTransferAccount{
			RootAccountId: abRootID, TargetAccountId: abTargetID,
			TargetScopeType: authv1.AccountScopeType_SCOPE_ROOT, SmartAccountAddress: abSmartAddr,
			RootUsername: "alice", SubaccountLabel: "ops",
		}},
		Tags:     []*authv1.AddressBookTag{{TagId: abTagID, Name: "hot"}},
		Revision: 1,
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertPublicIDs fails when a decimal ID leaks or a public ID is missing.
func assertPublicIDs(t *testing.T, what, got string, ids ...uint64) {
	t.Helper()
	for _, id := range ids {
		if want := `"` + codecs.FormatID(id) + `"`; !strings.Contains(got, want) {
			t.Errorf("%s: public ID %s missing in %s", what, want, got)
		}
		if decimal := `"` + strconv.FormatUint(id, 10) + `"`; strings.Contains(got, decimal) {
			t.Errorf("%s: decimal ID %s leaked in %s", what, decimal, got)
		}
	}
}

// A listed entry carries its destination, note, scope and times.
func TestAddressBookEntryCarriesDestinationNoteScopeAndTimes(t *testing.T) {
	got := mustJSON(t, ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{
		Entries: []*authv1.AddressBookEntry{abInternalEntry(), {
			AddressBookEntryId: 8, Kind: authv1.AddressBookEntryKind_EXTERNAL_CHAIN,
			Entry: &authv1.AddressBookEntry_External{External: &authv1.ExternalWithdrawAddress{PolychainChainId: 42161, Address: "0xabc"}},
		}},
	}))
	for _, want := range []string{"desk wallet", abSmartAddr, "2026-03-04T05:06:07.825341Z", "2026-03-04T05:06:08.825341Z", "SCOPE_SUBACCOUNT", "42161", "0xabc", `"ops"`} {
		if !strings.Contains(got, want) {
			t.Errorf("entry JSON lacks %q: %s", want, got)
		}
	}
	assertPublicIDs(t, "entry", got, abEntryID, abRootID, abTargetID, abSubID, abTagID)
}

// The view reads an entry with the same public ID as ListEntries, and every
// account, sub-account and tag ID in it is a public ID.
func TestAddressBookViewUsesPublicIDs(t *testing.T) {
	view := AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{
		Books: []*authv1.AddressBook{{Scope: abScope(), CallerRole: authv1.SubaccountRole_OWNER}},
		Entries: &authv1.AddressBookEntriesView{
			Internal: []*authv1.InternalAddressBookEntry{{
				AddressBookEntryId: abEntryID, Scope: abScope(), TagIds: []uint64{abTagID},
				RootAccountId: abRootID, TargetAccountId: abTargetID, CreatedAt: timestamppb.New(abCreated),
			}},
		},
		Tags:         []*authv1.AddressBookTagSummary{{TagId: abTagID, Name: "hot"}},
		ViewRevision: 3,
	})
	got := mustJSON(t, view)
	listed := ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{Entries: []*authv1.AddressBookEntry{abInternalEntry()}})
	if !strings.Contains(got, `"`+listed.Entries[0].AddressBookEntryID+`"`) {
		t.Errorf("view lacks ListEntries' entry ID %q: %s", listed.Entries[0].AddressBookEntryID, got)
	}
	assertPublicIDs(t, "view", got, abEntryID, abRootID, abTargetID, abSubID, abTagID)
}

// Counterparty, destination, whitelist and withdraw-whitelist reads use
// public IDs too.
func TestAddressBookListsAndWhitelistViewUsePublicIDs(t *testing.T) {
	counterparties := mustJSON(t, ListCounterpartiesFromProto(&authv1.ListTransferCounterpartiesResponse{
		Counterparties: []*authv1.TransferCounterparty{{
			CounterpartyId: abSubID, Scope: abScope(), AddressBookEntryId: abEntryID,
			Counterparty: &authv1.TransferCounterparty_Internal{Internal: &authv1.InternalTransferAccount{RootAccountId: abRootID, TargetAccountId: abTargetID}},
		}},
	}))
	assertPublicIDs(t, "counterparties", counterparties, abSubID, abEntryID, abRootID, abTargetID)
	destinations := mustJSON(t, ListDestinationsFromProto(&authv1.ListTransferDestinationsResponse{
		Destinations: []*authv1.TransferDestination{{Scope: abScope(), AddressBookEntry: abInternalEntry()}},
	}))
	assertPublicIDs(t, "destinations", destinations, abEntryID, abRootID, abTargetID, abSubID)
	whitelist := mustJSON(t, ListInternalWhitelistFromProto(&authv1.ListInternalTransferWhitelistEntriesResponse{
		Entries: []*authv1.InternalTransferWhitelistEntry{{EntryId: abEntryID, Scope: abScope(), RootAccountId: abRootID, TargetAccountId: abTargetID}},
	}))
	assertPublicIDs(t, "internal whitelist", whitelist, abEntryID, abRootID, abTargetID, abSubID)
	withdraw := mustJSON(t, WithdrawWhitelistViewFromGetProto(&authv1.GetWithdrawWhitelistViewResponse{
		View: &authv1.WithdrawWhitelistView{Scope: abScope()},
	}))
	assertPublicIDs(t, "withdraw whitelist", withdraw, abRootID, abSubID)
}

// The typed entry and view carry the same values field by field.
func TestAddressBookEntryAndViewTypedFields(t *testing.T) {
	entry := entryFromProto(abInternalEntry())
	wantScope := &models.AccountScope{ScopeType: "SCOPE_SUBACCOUNT", RootAccountID: codecs.FormatID(abRootID), SubAccountID: codecs.FormatID(abSubID)}
	if !reflect.DeepEqual(entry.Scope, wantScope) || entry.Note != "desk wallet" || entry.External != nil {
		t.Fatalf("entry %+v scope %+v", entry, entry.Scope)
	}
	if entry.CreatedAt == nil || !entry.CreatedAt.Equal(abCreated) || entry.UpdatedAt == nil || !entry.UpdatedAt.Equal(abCreated.Add(time.Second)) {
		t.Fatalf("times %v %v", entry.CreatedAt, entry.UpdatedAt)
	}
	wantInternal := &models.InternalTransferAccount{
		RootAccountID: codecs.FormatID(abRootID), TargetAccountID: codecs.FormatID(abTargetID), TargetScopeType: "SCOPE_ROOT",
		SmartAccountAddress: abSmartAddr, RootUsername: "alice", SubAccountLabel: "ops",
	}
	if !reflect.DeepEqual(entry.Internal, wantInternal) {
		t.Fatalf("internal %+v", entry.Internal)
	}
	if entry.AddressBookEntryID != codecs.FormatID(abEntryID) || entry.Kind != "INTERNAL_ACCOUNT" || entry.Revision != 1 || len(entry.Tags) != 1 {
		t.Fatalf("existing fields changed: %+v", entry)
	}
	// An unset time stays nil, never the Unix epoch.
	if bare := entryFromProto(&authv1.AddressBookEntry{AddressBookEntryId: 1}); bare.CreatedAt != nil || bare.Scope != nil || bare.Internal != nil {
		t.Fatalf("bare entry %+v", bare)
	}
	for name, decoded := range map[string]func() (models.AddressBookEntry, error){
		"create": func() (models.AddressBookEntry, error) {
			return EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{Entry: abInternalEntry()})
		},
		"update": func() (models.AddressBookEntry, error) {
			return EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{Entry: abInternalEntry()})
		},
	} {
		got, err := decoded()
		if err != nil || !reflect.DeepEqual(got, entry) {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}

	view := AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{
		Books: []*authv1.AddressBook{{Scope: abScope(), CallerRole: authv1.SubaccountRole_OWNER, Label: "ops"}},
		Entries: &authv1.AddressBookEntriesView{
			External: []*authv1.ExternalAddressBookEntry{{AddressBookEntryId: 8, PolychainChainId: 42161, Address: "0xabc", Revision: 4,
				WhitelistStatus: authv1.DestinationWhitelistStatus_DESTINATION_WHITELIST_ACTIVE}},
			Internal: []*authv1.InternalAddressBookEntry{{AddressBookEntryId: abEntryID, TagIds: []uint64{abTagID}, TargetAccountId: abTargetID}},
		},
		RecentDestinations: &authv1.AddressBookRecentDestinationsView{
			External: []*authv1.ExternalRecentDestination{{LastDirection: authv1.TransferCounterpartyDirection_WITHDRAW_TO, UseCount: 3}},
		},
		WithdrawWhitelist:           &authv1.WithdrawWhitelistView{ExternalWhitelistRequired: true, ActiveEntries: []*authv1.MirroredWithdrawWhitelistEntry{{CanonicalAddress: "0xabc"}}},
		RecentDestinationsTruncated: true,
		ViewRevision:                3,
	})
	if len(view.Books) != 1 || view.Books[0].CallerRole != "owner" || !reflect.DeepEqual(view.Books[0].Scope, wantScope) {
		t.Fatalf("books %+v", view.Books)
	}
	ext, in := view.Entries.External[0], view.Entries.Internal[0]
	if ext.AddressBookEntryID != codecs.FormatID(8) || ext.Address != "0xabc" || ext.PolychainChainID != 42161 || ext.Revision != 4 || ext.WhitelistStatus != "DESTINATION_WHITELIST_ACTIVE" {
		t.Fatalf("external view entry %+v", ext)
	}
	if in.AddressBookEntryID != entry.AddressBookEntryID || !reflect.DeepEqual(in.TagIDs, []string{entry.Tags[0].TagID}) || in.TargetAccountID != wantInternal.TargetAccountID {
		t.Fatalf("internal view entry %+v, want IDs of %+v", in, entry)
	}
	if r := view.RecentDestinations.External[0]; r.LastDirection != "WITHDRAW_TO" || r.UseCount != 3 || r.AddressBookEntryID != "" {
		t.Fatalf("recent %+v", r)
	}
	if !view.WithdrawWhitelist.ExternalWhitelistRequired || view.WithdrawWhitelist.ActiveEntries[0].CanonicalAddress != "0xabc" || !view.RecentDestinationsTruncated || view.ViewRevision != 3 {
		t.Fatalf("view %+v", view)
	}
	if books := ListBooksFromProto(&authv1.ListAddressBooksResponse{Books: []*authv1.AddressBook{{Scope: abScope(), CallerRole: authv1.SubaccountRole_TRADER}}}); books.Books[0].CallerRole != "trader" || books.Books[0].Scope.RootAccountID != codecs.FormatID(abRootID) {
		t.Fatalf("list books %+v", books)
	}
}
