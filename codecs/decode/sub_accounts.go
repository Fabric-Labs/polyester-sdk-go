package decode

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func subaccountStatusName(status authv1.SubaccountStatus) string {
	switch status {
	case authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE:
		return "active"
	case authv1.SubaccountStatus_SUBACCOUNT_STATUS_DISABLED:
		return "disabled"
	case authv1.SubaccountStatus_SUBACCOUNT_STATUS_UNSPECIFIED:
		return ""
	default:
		return fmt.Sprintf("UNKNOWN(%d)", status)
	}
}

// subaccountRoleName spells a role the same way on a sub-account, its
// members, its invites and an address book: lower case (owner,
// leveraged_trader), "" when unspecified.
func subaccountRoleName(role authv1.SubaccountRole) string {
	return codecs.ProtoEnumName(role, role.Number())
}

func subAccount(msg *authv1.Subaccount) models.SubAccount {
	if msg == nil {
		return models.SubAccount{}
	}
	var saltNonce *uint32
	if msg.SmartAccountSaltNonce != nil {
		value := msg.GetSmartAccountSaltNonce()
		saltNonce = &value
	}
	return models.SubAccount{
		SubaccountID: codecs.FormatUint64ID(msg.GetId()),
		// Unspecified (as on a removal) reads as "", never as a default role.
		Role:                subaccountRoleName(msg.GetRole()),
		Removed:             msg.GetRemoved(),
		Label:               msg.GetLabel(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		Status:              subaccountStatusName(msg.GetStatus()),
		Revision:            msg.GetRevision(),
		UpdatedAt:           optionalTime(msg.GetUpdatedAt()),
		Icon:                msg.GetIcon(),
		Color:               msg.GetColor(),
		OwnerUsername:       msg.GetOwnerUsername(),
		OwnerAvatarURL:      msg.GetOwnerAvatarUrl(),

		OwnerRootSmartAccountAddress: msg.GetOwnerRootSmartAccountAddress(),
		// Zero means no policy is attached.
		SubaccountPolicyID:    optionalID(msg.GetSubaccountPolicyId()),
		RequireMemberMFA:      msg.GetRequireMemberMfa(),
		SmartAccountSaltNonce: saltNonce,
	}
}

// optionalTime reads a timestamp the server may leave out as nil.
func optionalTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	value := ts.AsTime()
	return &value
}

// SubaccountMessageFromProto decodes one subaccount message.
func SubaccountMessageFromProto(msg *authv1.Subaccount) models.SubAccount { return subAccount(msg) }

// SubaccountsListFromProto decodes the account's sub-accounts. A sub-account
// without its ID returns a *errors.ResponseContractError.
func SubaccountsListFromProto(msg *authv1.ListSubaccountsResponse) (models.SubAccountsList, error) {
	out := make([]models.SubAccount, 0, len(msg.GetSubaccounts()))
	for _, s := range msg.GetSubaccounts() {
		if err := requireID("ListSubaccounts", "id", s.GetId()); err != nil {
			return models.SubAccountsList{}, err
		}
		out = append(out, subAccount(s))
	}
	return models.SubAccountsList{Subaccounts: out}, nil
}

// requireInviteIDs refuses an invite without its ID or its sub-account's ID.
func requireInviteIDs(operation string, invites []*authv1.SubaccountInvite) error {
	for _, inv := range invites {
		if err := requireID(operation, "invite id", inv.GetId()); err != nil {
			return err
		}
		if err := requireID(operation, "invite subaccount_id", inv.GetSubaccountId()); err != nil {
			return err
		}
	}
	return nil
}

// GetSubaccountFromProto decodes one sub-account with its expansions. A reply
// without its sub-account, or a sub-account, API key, member, invite or
// attached policy without its ID, returns a *errors.ResponseContractError.
func GetSubaccountFromProto(msg *authv1.GetSubaccountResponse) (models.GetSubaccountResult, error) {
	if err := requireID("GetSubaccount", "subaccount id", msg.GetSubaccount().GetId()); err != nil {
		return models.GetSubaccountResult{}, err
	}
	if err := requireAPIKeyIDs("GetSubaccount", msg.GetApiKeys()...); err != nil {
		return models.GetSubaccountResult{}, err
	}
	if err := requireMemberIDs("GetSubaccount", msg.GetMembers()); err != nil {
		return models.GetSubaccountResult{}, err
	}
	if err := requireInviteIDs("GetSubaccount", msg.GetInvites()); err != nil {
		return models.GetSubaccountResult{}, err
	}
	if policy := msg.GetPolicy(); policy != nil {
		if err := requireID("GetSubaccount", "policy id", policy.GetId()); err != nil {
			return models.GetSubaccountResult{}, err
		}
	}
	s := subAccount(msg.GetSubaccount())
	out := models.GetSubaccountResult{Subaccount: &s}
	if len(msg.GetApiKeys()) > 0 {
		out.APIKeys = apiKeys(msg.GetApiKeys())
	}
	if len(msg.GetMembers()) > 0 {
		out.Members = subAccountMembers(msg.GetMembers())
	}
	if len(msg.GetInvites()) > 0 {
		out.Invites = subAccountInvites(msg.GetInvites())
	}
	// Nil when none is attached: the server leaves policy unset.
	out.Policy = SubaccountPolicyMessageFromProto(msg.GetPolicy())
	if msg.GetBalances() != nil {
		balances := BalancesListFromProto(msg.GetBalances())
		out.Balances = &balances
	}
	return out, nil
}

func subAccountMembers(members []*authv1.SubaccountMemberView) []models.SubAccountMember {
	out := make([]models.SubAccountMember, 0, len(members))
	for _, m := range members {
		out = append(out, models.SubAccountMember{
			GranteeAccountID: codecs.FormatUint64ID(m.GetAccountId()), Role: subaccountRoleName(m.GetRole()),
			Username: m.GetUsername(), SmartAccountAddress: m.GetSmartAccountAddress(),
			AvatarURL: m.GetAvatarUrl(), MFAEnrolled: m.GetMfaEnrolled(),
		})
	}
	return out
}

// SubaccountMembersListFromProto decodes sub-account members. A member
// without its account ID returns a *errors.ResponseContractError.
func SubaccountMembersListFromProto(msg *authv1.ListSubaccountMembersResponse) (models.SubAccountMembersList, error) {
	if err := requireMemberIDs("ListSubaccountMembers", msg.GetMembers()); err != nil {
		return models.SubAccountMembersList{}, err
	}
	return models.SubAccountMembersList{Members: subAccountMembers(msg.GetMembers())}, nil
}

// requireMemberIDs refuses a member without its account ID.
func requireMemberIDs(operation string, members []*authv1.SubaccountMemberView) error {
	for _, m := range members {
		if err := requireID(operation, "member account_id", m.GetAccountId()); err != nil {
			return err
		}
	}
	return nil
}

func inviteFromProto(inv *authv1.SubaccountInvite) models.SubAccountInvite {
	if inv == nil {
		return models.SubAccountInvite{}
	}
	return models.SubAccountInvite{
		InviteID:         codecs.FormatUint64ID(inv.GetId()),
		SubaccountID:     codecs.FormatUint64ID(inv.GetSubaccountId()),
		GranteeAccountID: codecs.FormatUint64ID(inv.GetGranteeAccountId()),
		InviterAccountID: optionalID(inv.GetInviterAccountId()),
		Role:             subaccountRoleName(inv.GetRole()), Status: inv.GetStatus().String(),
		CreatedAt:       optionalTime(inv.GetCreatedAt()),
		RespondedAt:     optionalTime(inv.GetRespondedAt()),
		GranteeUsername: inv.GetGranteeUsername(),
		InviterUsername: inv.GetInviterUsername(),
		SubaccountLabel: inv.GetSubaccountLabel(),

		InviterRootSmartAccountAddress: inv.GetInviterRootSmartAccountAddress(),
		GranteeRootSmartAccountAddress: inv.GetGranteeRootSmartAccountAddress(),
		RequireMemberMFA:               inv.GetRequireMemberMfa(),
	}
}

func subAccountInvites(invites []*authv1.SubaccountInvite) []models.SubAccountInvite {
	out := make([]models.SubAccountInvite, 0, len(invites))
	for _, inv := range invites {
		out = append(out, inviteFromProto(inv))
	}
	return out
}

// SubaccountInvitesListFromProto decodes sub-account invites. An invite
// without its ID or its sub-account's ID returns a
// *errors.ResponseContractError.
func SubaccountInvitesListFromProto(msg *authv1.ListSubaccountInvitesResponse) (models.SubAccountInvitesList, error) {
	if err := requireInviteIDs("ListSubaccountInvites", msg.GetInvites()); err != nil {
		return models.SubAccountInvitesList{}, err
	}
	return models.SubAccountInvitesList{Invites: subAccountInvites(msg.GetInvites())}, nil
}

func SubaccountActivityListFromProto(msg *authv1.ListSubaccountEventsResponse) models.SubAccountActivityList {
	out := make([]models.SubAccountActivityEvent, 0, len(msg.GetEvents()))
	for _, e := range msg.GetEvents() {
		var tsMs int64
		if e.GetCreatedAt() != nil {
			tsMs = e.GetCreatedAt().AsTime().UnixMilli()
		}
		out = append(out, models.SubAccountActivityEvent{
			EventType: strings.ToLower(strings.TrimPrefix(e.GetEntityKind().String(), "ACTIVITY_ENTITY_")) +
				":" + strings.ToLower(strings.TrimPrefix(e.GetEventAction().String(), "ACTIVITY_ACTION_")),
			TsMs: tsMs,
		})
	}
	return models.SubAccountActivityList{Events: out, NextPageToken: msg.GetNextPageToken()}
}
