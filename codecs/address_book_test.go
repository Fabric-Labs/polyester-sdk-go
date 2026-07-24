package codecs

import (
	"strings"
	"testing"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestAddressBookEntryKindFromLabel(t *testing.T) {
	if got := AddressBookEntryKindFromLabel("external"); got != authv1.AddressBookEntryKind_EXTERNAL_CHAIN {
		t.Fatalf("external=%v", got)
	}
	if got := AddressBookEntryKindFromLabel("internal_account"); got != authv1.AddressBookEntryKind_INTERNAL_ACCOUNT {
		t.Fatalf("internal=%v", got)
	}
	if got := AddressBookEntryKindFromLabel("nope"); got != authv1.AddressBookEntryKind_ENTRY_KIND_UNSPECIFIED {
		t.Fatalf("unknown=%v", got)
	}
}

func TestTransferCounterpartyDirectionFromLabel(t *testing.T) {
	cases := map[string]authv1.TransferCounterpartyDirection{
		"deposit_from": authv1.TransferCounterpartyDirection_DEPOSIT_FROM,
		"incoming":     authv1.TransferCounterpartyDirection_DEPOSIT_FROM,
		"withdraw_to":  authv1.TransferCounterpartyDirection_WITHDRAW_TO,
		"outgoing":     authv1.TransferCounterpartyDirection_WITHDRAW_TO,
	}
	for label, want := range cases {
		if got := TransferCounterpartyDirectionFromLabel(label); got != want {
			t.Fatalf("%s: got=%v want=%v", label, got, want)
		}
		if got := TransferCounterpartyDirectionFromLabel(strings.ToUpper(label)); got != want {
			t.Fatalf("%s upper: got=%v want=%v", label, got, want)
		}
	}
}
