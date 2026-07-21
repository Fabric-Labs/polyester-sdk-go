package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// AddressBookEntryPatch carries optional address-book entry update fields.
// Nil pointer = omit; non-nil (including empty string/slice) = set and include in the mask.
type AddressBookEntryPatch struct {
	ExpectedRevision uint64
	Label            *string
	Note             *string
	TagIDs           *[]string
}

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

func AddressBookTagInputToProto(name, color string) *authv1.AddressBookTagInput {
	return &authv1.AddressBookTagInput{Name: name, Color: color}
}

func CreateEntryExternalToProto(polychainChainID uint32, address string) *authv1.CreateAddressBookEntryRequest_External {
	return &authv1.CreateAddressBookEntryRequest_External{
		External: &authv1.ExternalWithdrawAddress{PolychainChainId: polychainChainID, Address: address},
	}
}

func CreateEntryInternalToProto(smartAccountAddress string) *authv1.CreateAddressBookEntryRequest_Internal {
	return &authv1.CreateAddressBookEntryRequest_Internal{
		Internal: &authv1.RequestedInternalTransferAccount{SmartAccountAddress: smartAccountAddress},
	}
}

func RequireExternalEntry(polychainChainID *uint32, address string) error {
	if polychainChainID == nil || address == "" {
		return &errors.ValidationError{Msg: "external entries require polychain_chain_id and address"}
	}
	return nil
}

// BuildUpdateAddressBookEntryRequest builds a durable address-book entry update request.
func BuildUpdateAddressBookEntryRequest(entryID uint64, patch AddressBookEntryPatch) (*authv1.UpdateAddressBookEntryRequest, error) {
	if err := requirePositiveRevision(patch.ExpectedRevision); err != nil {
		return nil, err
	}
	spec := &authv1.AddressBookEntryUpdateSpec{}
	var paths []string
	if patch.Label != nil {
		spec.Label = *patch.Label
		paths = append(paths, "label")
	}
	if patch.Note != nil {
		spec.Note = *patch.Note
		paths = append(paths, "note")
	}
	if patch.TagIDs != nil {
		ids := make([]uint64, 0, len(*patch.TagIDs))
		for _, item := range *patch.TagIDs {
			id, err := IDToInt(item, "tag_id")
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		spec.TagIds = ids
		paths = append(paths, "tag_ids")
	}
	mask, err := newUpdateMask(paths)
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateAddressBookEntryRequest{
		AddressBookEntryId: entryID,
		Entry:              spec,
		UpdateMask:         mask,
		ExpectedRevision:   patch.ExpectedRevision,
	}, nil
}
