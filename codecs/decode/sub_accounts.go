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
		SubaccountID:        codecs.FormatUint64ID(msg.GetId()),
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
	return models.GetSubaccountResult{Subaccount: &s}
}

func SubaccountMembersListFromProto(msg *authv1.ListSubaccountMembersResponse) models.SubAccountMembersList {
	out := make([]models.SubAccountMember, 0, len(msg.GetMembers()))
	for _, m := range msg.GetMembers() {
		out = append(out, models.SubAccountMember{
			GranteeAccountID: codecs.FormatUint64ID(m.GetAccountId()), Role: m.GetRole().String(),
		})
	}
	return models.SubAccountMembersList{Members: out}
}

func inviteFromProto(inv *authv1.SubaccountInvite) models.SubAccountInvite {
	if inv == nil {
		return models.SubAccountInvite{}
	}
	return models.SubAccountInvite{
		InviteID:         codecs.FormatUint64ID(inv.GetId()),
		GranteeAccountID: codecs.FormatUint64ID(inv.GetGranteeAccountId()),
		Role:             inv.GetRole().String(), Status: inv.GetStatus().String(),
	}
}

func SubaccountInvitesListFromProto(msg *authv1.ListSubaccountInvitesResponse) models.SubAccountInvitesList {
	out := make([]models.SubAccountInvite, 0, len(msg.GetInvites()))
	for _, inv := range msg.GetInvites() {
		out = append(out, inviteFromProto(inv))
	}
	return models.SubAccountInvitesList{Invites: out}
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
