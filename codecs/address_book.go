package codecs

import (
	"strings"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func AddressBookEntryKindFromLabel(kind string) authv1.AddressBookEntryKind {
	switch strings.ToLower(kind) {
	case "external", "external_chain":
		return authv1.AddressBookEntryKind_EXTERNAL_CHAIN
	case "internal", "internal_account":
		return authv1.AddressBookEntryKind_INTERNAL_ACCOUNT
	default:
		return authv1.AddressBookEntryKind_ENTRY_KIND_UNSPECIFIED
	}
}

func TransferCounterpartyDirectionFromLabel(direction string) authv1.TransferCounterpartyDirection {
	switch strings.ToLower(direction) {
	case "deposit_from", "incoming":
		return authv1.TransferCounterpartyDirection_DEPOSIT_FROM
	case "withdraw_to", "outgoing":
		return authv1.TransferCounterpartyDirection_WITHDRAW_TO
	default:
		return authv1.TransferCounterpartyDirection_TRANSFER_COUNTERPARTY_DIRECTION_UNSPECIFIED
	}
}
