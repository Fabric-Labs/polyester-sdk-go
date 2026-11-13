package codecs

import (
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// AddressBookEntryPatch carries optional address-book entry update fields.
// Nil pointer = omit; non-nil (including empty string/slice) = set and include in the mask.
type AddressBookEntryPatch struct {
	ExpectedRevision uint64
	Label            *string
	Note             *string
	TagIDs           *[]string
	NewTags          *[]models.AddressBookTagInput
}

// AddressBookEntryKindFromLabel reads an entry kind filter, in any case:
// external or external_chain, internal or internal_account. Any other label
// is refused: the contract reads an unspecified kind as no filter.
func AddressBookEntryKindFromLabel(kind string) (authv1.AddressBookEntryKind, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "external", "external_chain":
		return authv1.AddressBookEntryKind_EXTERNAL_CHAIN, nil
	case "internal", "internal_account":
		return authv1.AddressBookEntryKind_INTERNAL_ACCOUNT, nil
	default:
		return authv1.AddressBookEntryKind_ENTRY_KIND_UNSPECIFIED, &errors.ValidationError{
			Msg: "kind must be external, external_chain, internal or internal_account; got " + strconv.Quote(kind),
		}
	}
}

// TransferCounterpartyDirectionFromLabel reads a counterparty direction
// filter, in any case: deposit_from (or incoming), withdraw_to (or
// outgoing), internal_transfer_from or internal_transfer_to. Any other label
// is refused: the contract reads an unspecified direction as no filter.
func TransferCounterpartyDirectionFromLabel(direction string) (authv1.TransferCounterpartyDirection, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "deposit_from", "incoming":
		return authv1.TransferCounterpartyDirection_DEPOSIT_FROM, nil
	case "withdraw_to", "outgoing":
		return authv1.TransferCounterpartyDirection_WITHDRAW_TO, nil
	case "internal_transfer_from":
		return authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_FROM, nil
	case "internal_transfer_to":
		return authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_TO, nil
	default:
		return authv1.TransferCounterpartyDirection_TRANSFER_COUNTERPARTY_DIRECTION_UNSPECIFIED, &errors.ValidationError{
			Msg: "direction must be deposit_from, withdraw_to, internal_transfer_from or internal_transfer_to (incoming and outgoing are aliases); got " + strconv.Quote(direction),
		}
	}
}

func AddressBookTagInputToProto(name, color string) *authv1.AddressBookTagInput {
	return &authv1.AddressBookTagInput{Name: name, Color: color}
}

func tagInputsToProto(tags []models.AddressBookTagInput) []*authv1.AddressBookTagInput {
	if len(tags) == 0 {
		return nil
	}
	out := make([]*authv1.AddressBookTagInput, 0, len(tags))
	for _, tag := range tags {
		out = append(out, AddressBookTagInputToProto(tag.Name, tag.Color))
	}
	return out
}

func parseTagIDs(tagIDs []string) ([]uint64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	out := make([]uint64, 0, len(tagIDs))
	for _, item := range tagIDs {
		id, err := IDToInt(item, "tag_id")
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
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

// CreateAddressBookEntryToProto encodes a create-entry request.
// Exactly one destination is required: external (address + polychain_chain_id)
// or internal (smart_account_address).
func CreateAddressBookEntryToProto(
	label, note string,
	externalAddress *string,
	polychainChainID *uint32,
	smartAccountAddress *string,
	tagIDs []string,
	newTags []models.AddressBookTagInput,
) (*authv1.CreateAddressBookEntryRequest, error) {
	external := ""
	if externalAddress != nil {
		external = strings.TrimSpace(*externalAddress)
	}
	internal := ""
	if smartAccountAddress != nil {
		internal = strings.TrimSpace(*smartAccountAddress)
	}
	hasExternal := external != ""
	hasInternal := internal != ""
	if hasExternal == hasInternal {
		return nil, &errors.ValidationError{Msg: "exactly one destination is required: external address or smart_account_address"}
	}

	req := &authv1.CreateAddressBookEntryRequest{Label: label, Note: note}
	if hasExternal {
		if polychainChainID == nil || *polychainChainID == 0 {
			return nil, &errors.ValidationError{Msg: "external entries require polychain_chain_id and address"}
		}
		req.Entry = CreateEntryExternalToProto(*polychainChainID, external)
	} else {
		req.Entry = CreateEntryInternalToProto(internal)
	}
	ids, err := parseTagIDs(tagIDs)
	if err != nil {
		return nil, err
	}
	req.TagIds = ids
	req.NewTags = tagInputsToProto(newTags)
	return req, nil
}

func requirePositiveRevision(rev uint64) error {
	if rev == 0 {
		return &errors.ValidationError{Msg: "expected_revision must be positive"}
	}
	return nil
}

func newUpdateMask(paths []string) (*fieldmaskpb.FieldMask, error) {
	if len(paths) == 0 {
		return nil, &errors.ValidationError{Msg: "update_mask must be non-empty"}
	}
	return &fieldmaskpb.FieldMask{Paths: paths}, nil
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
		ids, err := parseTagIDs(*patch.TagIDs)
		if err != nil {
			return nil, err
		}
		spec.TagIds = ids
		paths = append(paths, "tag_ids")
	}
	if patch.NewTags != nil {
		spec.NewTags = tagInputsToProto(*patch.NewTags)
		paths = append(paths, "new_tags")
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
