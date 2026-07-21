package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// SubaccountPatch carries optional subaccount update fields.
// Nil pointer = omit; non-nil (including empty string) = set and include in the mask.
type SubaccountPatch struct {
	ExpectedRevision uint64
	Label            *string
	Icon             *string
	Color            *string
	Status           *string
}

// SubaccountRoleFromLabel maps a role label to the proto enum.
func SubaccountRoleFromLabel(value string) (authv1.SubaccountRole, error) {
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	normalized = strings.TrimPrefix(normalized, "subaccount_role_")
	enumName := strings.ToUpper(normalized)
	if v, ok := authv1.SubaccountRole_value[enumName]; ok && v != 0 {
		return authv1.SubaccountRole(v), nil
	}
	return 0, &errors.ValidationError{Msg: "unknown subaccount role: " + value}
}

// SubaccountInviteActionFromLabel maps invite action labels to the proto enum.
func SubaccountInviteActionFromLabel(value string) (authv1.SubaccountInviteAction, error) {
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	switch normalized {
	case "accept":
		return authv1.SubaccountInviteAction_SUBACCOUNT_INVITE_ACTION_ACCEPT, nil
	case "decline":
		return authv1.SubaccountInviteAction_SUBACCOUNT_INVITE_ACTION_DECLINE, nil
	case "cancel":
		return authv1.SubaccountInviteAction_SUBACCOUNT_INVITE_ACTION_CANCEL, nil
	}
	if v, ok := authv1.SubaccountInviteAction_value["SUBACCOUNT_INVITE_ACTION_"+strings.ToUpper(normalized)]; ok && v != 0 {
		return authv1.SubaccountInviteAction(v), nil
	}
	return 0, &errors.ValidationError{Msg: "invite action must be 'accept', 'decline', or 'cancel'"}
}

// BuildUpdateSubaccountRequest builds a durable subaccount update request.
func BuildUpdateSubaccountRequest(subaccountID uint64, patch SubaccountPatch) (*authv1.UpdateSubaccountRequest, error) {
	if err := requirePositiveRevision(patch.ExpectedRevision); err != nil {
		return nil, err
	}
	spec := &authv1.SubaccountUpdateSpec{}
	var paths []string
	if patch.Label != nil {
		spec.Label = *patch.Label
		paths = append(paths, "label")
	}
	if patch.Icon != nil {
		spec.Icon = *patch.Icon
		paths = append(paths, "icon")
	}
	if patch.Color != nil {
		spec.Color = *patch.Color
		paths = append(paths, "color")
	}
	if patch.Status != nil {
		spec.Status = *patch.Status
		paths = append(paths, "status")
	}
	mask, err := newUpdateMask(paths)
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateSubaccountRequest{
		SubaccountId:     subaccountID,
		Subaccount:       spec,
		UpdateMask:       mask,
		ExpectedRevision: patch.ExpectedRevision,
	}, nil
}
