package codecs

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestAddressBookEntryKindFromLabel(t *testing.T) {
	for label, want := range map[string]authv1.AddressBookEntryKind{
		"external": authv1.AddressBookEntryKind_EXTERNAL_CHAIN, "EXTERNAL_CHAIN": authv1.AddressBookEntryKind_EXTERNAL_CHAIN,
		"internal": authv1.AddressBookEntryKind_INTERNAL_ACCOUNT, "internal_account": authv1.AddressBookEntryKind_INTERNAL_ACCOUNT,
	} {
		if got, err := AddressBookEntryKindFromLabel(label); err != nil || got != want {
			t.Fatalf("%s: got=%v err=%v want=%v", label, got, err, want)
		}
	}
	for _, label := range []string{"nope", "", "entry_kind_unspecified", "unspecified", "scope_root"} {
		var validation *sdkerrors.ValidationError
		if _, err := AddressBookEntryKindFromLabel(label); !errors.As(err, &validation) {
			t.Fatalf("%q: err=%v, want a validation error", label, err)
		}
	}
}

func TestTransferCounterpartyDirectionFromLabel(t *testing.T) {
	cases := map[string]authv1.TransferCounterpartyDirection{
		"deposit_from":           authv1.TransferCounterpartyDirection_DEPOSIT_FROM,
		"incoming":               authv1.TransferCounterpartyDirection_DEPOSIT_FROM,
		"withdraw_to":            authv1.TransferCounterpartyDirection_WITHDRAW_TO,
		"outgoing":               authv1.TransferCounterpartyDirection_WITHDRAW_TO,
		"internal_transfer_from": authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_FROM,
		"internal_transfer_to":   authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_TO,
	}
	for label, want := range cases {
		if got, err := TransferCounterpartyDirectionFromLabel(label); err != nil || got != want {
			t.Fatalf("%s: got=%v err=%v want=%v", label, got, err, want)
		}
		if got, err := TransferCounterpartyDirectionFromLabel(strings.ToUpper(label)); err != nil || got != want {
			t.Fatalf("%s upper: got=%v err=%v want=%v", label, got, err, want)
		}
	}
	for _, label := range []string{"sideways", "", "transfer_counterparty_direction_unspecified", "scope_root"} {
		var validation *sdkerrors.ValidationError
		if _, err := TransferCounterpartyDirectionFromLabel(label); !errors.As(err, &validation) {
			t.Fatalf("%q: err=%v, want a validation error", label, err)
		}
	}
}

func TestCreateAddressBookEntryToProtoIncludesNewTags(t *testing.T) {
	addr := "0xabc"
	chainID := uint32(56)
	req, err := CreateAddressBookEntryToProto(
		"label",
		"note",
		&addr,
		&chainID,
		nil,
		nil,
		[]models.AddressBookTagInput{{Name: "hot", Color: "#ff0000"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if req.GetExternal() == nil || req.GetExternal().GetAddress() != addr || req.GetExternal().GetPolychainChainId() != chainID {
		t.Fatalf("external=%+v", req.GetExternal())
	}
	if req.GetInternal() != nil {
		t.Fatalf("internal should be unset: %+v", req.GetInternal())
	}
	if len(req.GetNewTags()) != 1 || req.GetNewTags()[0].GetName() != "hot" || req.GetNewTags()[0].GetColor() != "#ff0000" {
		t.Fatalf("new_tags=%+v", req.GetNewTags())
	}
}

func TestCreateAddressBookEntryToProtoRequiresExactlyOneDestination(t *testing.T) {
	addr := "0xabc"
	chainID := uint32(1)
	smart := "0xsmart"
	if _, err := CreateAddressBookEntryToProto("l", "", nil, nil, nil, nil, nil); err == nil {
		t.Fatal("expected rejection when both destinations are omitted")
	}
	if _, err := CreateAddressBookEntryToProto("l", "", &addr, &chainID, &smart, nil, nil); err == nil {
		t.Fatal("expected rejection when both destinations are set")
	}
}

func TestBuildUpdateAddressBookEntryRequestNewTagsMask(t *testing.T) {
	req, err := BuildUpdateAddressBookEntryRequest(7, AddressBookEntryPatch{
		ExpectedRevision: 3,
		NewTags:          &[]models.AddressBookTagInput{{Name: "append"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := req.GetUpdateMask().GetPaths(); len(got) != 1 || got[0] != "new_tags" {
		t.Fatalf("mask=%v", got)
	}
	if req.GetEntry().GetTagIds() != nil {
		t.Fatalf("tag_ids should be omitted: %v", req.GetEntry().GetTagIds())
	}
	if len(req.GetEntry().GetNewTags()) != 1 || req.GetEntry().GetNewTags()[0].GetName() != "append" {
		t.Fatalf("new_tags=%+v", req.GetEntry().GetNewTags())
	}
}

func TestBuildUpdateAddressBookEntryRequestTagIDsAndNewTags(t *testing.T) {
	tagID := FormatID(99)
	req, err := BuildUpdateAddressBookEntryRequest(7, AddressBookEntryPatch{
		ExpectedRevision: 4,
		TagIDs:           &[]string{tagID},
		NewTags:          &[]models.AddressBookTagInput{{Name: "extra"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := req.GetUpdateMask().GetPaths()
	if len(got) != 2 || got[0] != "tag_ids" || got[1] != "new_tags" {
		t.Fatalf("mask=%v", got)
	}
	if len(req.GetEntry().GetTagIds()) != 1 || req.GetEntry().GetTagIds()[0] != 99 {
		t.Fatalf("tag_ids=%v", req.GetEntry().GetTagIds())
	}
}

func TestBuildUpdateAddressBookEntryRequestRejectsEmptyAndZeroRevision(t *testing.T) {
	_, err := BuildUpdateAddressBookEntryRequest(7, AddressBookEntryPatch{ExpectedRevision: 1})
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("empty update: got %T: %v", err, err)
	}
	label := "keep"
	_, err = BuildUpdateAddressBookEntryRequest(7, AddressBookEntryPatch{
		ExpectedRevision: 0,
		Label:            &label,
	})
	if !errors.As(err, &validationErr) {
		t.Fatalf("zero revision: got %T: %v", err, err)
	}
}
