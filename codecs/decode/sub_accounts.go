package decode

import (
	"fmt"
	"strings"
	"time"

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
	var updatedAt *time.Time
	if ts := msg.GetUpdatedAt(); ts != nil {
		value := ts.AsTime()
		updatedAt = &value
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
		UpdatedAt:           updatedAt,
	}
}

// SubaccountMessageFromProto decodes one subaccount message.
func SubaccountMessageFromProto(msg *authv1.Subaccount) models.SubAccount { return subAccount(msg) }

func SubaccountsListFromProto(msg *authv1.ListSubaccountsResponse) models.SubAccountsList {
	out := make([]models.SubAccount, 0, len(msg.GetSubaccounts()))
	for _, s := range msg.GetSubaccounts() {
		out = append(out, subAccount(s))
	}
	return models.SubAccountsList{Subaccounts: out}
}

func GetSubaccountFromProto(msg *authv1.GetSubaccountResponse) models.GetSubaccountResult {
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
	return out
}

func subAccountMembers(members []*authv1.SubaccountMemberView) []models.SubAccountMember {
	out := make([]models.SubAccountMember, 0, len(members))
	for _, m := range members {
		out = append(out, models.SubAccountMember{
			GranteeAccountID: codecs.FormatUint64ID(m.GetAccountId()), Role: subaccountRoleName(m.GetRole()),
		})
	}
	return out
}

func SubaccountMembersListFromProto(msg *authv1.ListSubaccountMembersResponse) models.SubAccountMembersList {
	return models.SubAccountMembersList{Members: subAccountMembers(msg.GetMembers())}
}

func inviteFromProto(inv *authv1.SubaccountInvite) models.SubAccountInvite {
	if inv == nil {
		return models.SubAccountInvite{}
	}
	return models.SubAccountInvite{
		InviteID:         codecs.FormatUint64ID(inv.GetId()),
		GranteeAccountID: codecs.FormatUint64ID(inv.GetGranteeAccountId()),
		Role:             subaccountRoleName(inv.GetRole()), Status: inv.GetStatus().String(),
	}
}

func subAccountInvites(invites []*authv1.SubaccountInvite) []models.SubAccountInvite {
	out := make([]models.SubAccountInvite, 0, len(invites))
	for _, inv := range invites {
		out = append(out, inviteFromProto(inv))
	}
	return out
}

func SubaccountInvitesListFromProto(msg *authv1.ListSubaccountInvitesResponse) models.SubAccountInvitesList {
	return models.SubAccountInvitesList{Invites: subAccountInvites(msg.GetInvites())}
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
