package decode

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestEntryFromProtoMapsTags(t *testing.T) {
	got := entryFromProto(&authv1.AddressBookEntry{
		AddressBookEntryId: 7,
		Label:              "saved",
		Kind:               authv1.AddressBookEntryKind_EXTERNAL_CHAIN,
		Revision:           2,
		Tags: []*authv1.AddressBookTag{
			{TagId: 11, Name: "hot", Color: "#f00"},
		},
	})
	if got.AddressBookEntryID != codecs.FormatUint64ID(7) || got.Label != "saved" || got.Revision != 2 {
		t.Fatalf("entry=%+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "hot" || got.Tags[0].Color != "#f00" || got.Tags[0].TagID != codecs.FormatUint64ID(11) {
		t.Fatalf("tags=%+v", got.Tags)
	}
}

func TestEntryFromCreateProtoFailsClosedWhenMissing(t *testing.T) {
	_, err := EntryFromCreateProto(nil)
	assertContract(t, err, "CreateAddressBookEntry")
	_, err = EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{})
	assertContract(t, err, "CreateAddressBookEntry")
}

func TestEntryFromUpdateProtoFailsClosedWhenMissing(t *testing.T) {
	_, err := EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{})
	assertContract(t, err, "UpdateAddressBookEntry")
}

func TestTagFromCreateAndUpdateProtoFailsClosedWhenMissing(t *testing.T) {
	_, err := TagFromCreateProto(&authv1.CreateAddressBookTagResponse{})
	assertContract(t, err, "CreateAddressBookTag")
	_, err = TagFromUpdateProto(&authv1.UpdateAddressBookTagResponse{})
	assertContract(t, err, "UpdateAddressBookTag")
}

func TestTagFromCreateProtoMapsFields(t *testing.T) {
	got, err := TagFromCreateProto(&authv1.CreateAddressBookTagResponse{
		Tag: &authv1.AddressBookTag{TagId: 3, Name: "cold", Color: "blue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TagID != codecs.FormatUint64ID(3) || got.Name != "cold" || got.Color != "blue" {
		t.Fatalf("tag=%+v", got)
	}
}

func TestAddressBookViewFromProtoMapsViewRevision(t *testing.T) {
	got := AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{ViewRevision: 17})
	if got.ViewRevision != 17 {
		t.Fatalf("view_revision=%d", got.ViewRevision)
	}
	if got.Raw == nil {
		t.Fatal("expected raw view map")
	}
}

func TestAddressBookInvalidationFromProtoMapsViewRevision(t *testing.T) {
	got := AddressBookInvalidationFromProto(&authv1.AddressBookViewInvalidated{
		ViewRevision: 9,
	})
	if got.ViewRevision != 9 {
		t.Fatalf("view_revision=%d", got.ViewRevision)
	}
}

func assertContract(t *testing.T, err error, operation string) {
	t.Helper()
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
	if contractErr.Operation != operation {
		t.Fatalf("operation=%q want %q", contractErr.Operation, operation)
	}
}
