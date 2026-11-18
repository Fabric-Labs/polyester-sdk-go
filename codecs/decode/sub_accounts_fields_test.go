package decode

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Contract subaccounts.proto: SubaccountInvite carries subaccount_id,
// inviter_account_id, created_at, responded_at and its display fields;
// SubaccountMemberView carries mfa_enrolled and the member's display fields;
// Subaccount carries subaccount_policy_id, require_member_mfa,
// smart_account_salt_nonce and its display fields. Every read keeps them.
func TestSubaccountReadsKeepEveryContractField(t *testing.T) {
	created := time.Date(2026, 11, 18, 1, 2, 3, 0, time.UTC)
	responded := created.Add(time.Hour)
	invite := &authv1.SubaccountInvite{
		Id: 4, SubaccountId: 2, GranteeAccountId: 5, InviterAccountId: 7,
		Role: authv1.SubaccountRole_TRADER, Status: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_ACCEPTED,
		CreatedAt: timestamppb.New(created), RespondedAt: timestamppb.New(responded),
		GranteeUsername: "bob", InviterUsername: "alice", SubaccountLabel: "desk",
		InviterRootSmartAccountAddress: "0x1111111111111111111111111111111111111111",
		GranteeRootSmartAccountAddress: "0x2222222222222222222222222222222222222222",
		RequireMemberMfa:               true,
	}
	wantInvite := models.SubAccountInvite{
		InviteID: codecs.FormatID(4), SubaccountID: codecs.FormatID(2),
		GranteeAccountID: codecs.FormatID(5), InviterAccountID: codecs.FormatID(7),
		Role: "trader", Status: "SUBACCOUNT_INVITE_STATUS_ACCEPTED",
		CreatedAt: &created, RespondedAt: &responded,
		GranteeUsername: "bob", InviterUsername: "alice", SubaccountLabel: "desk",
		InviterRootSmartAccountAddress: "0x1111111111111111111111111111111111111111",
		GranteeRootSmartAccountAddress: "0x2222222222222222222222222222222222222222",
		RequireMemberMFA:               true,
	}
	member := &authv1.SubaccountMemberView{
		AccountId: 3, Role: authv1.SubaccountRole_ADMIN, Username: "carol",
		SmartAccountAddress: "0x3333333333333333333333333333333333333333",
		AvatarUrl:           "https://example.com/a.png", MfaEnrolled: true,
	}
	wantMember := models.SubAccountMember{
		GranteeAccountID: codecs.FormatID(3), Role: "admin", Username: "carol",
		SmartAccountAddress: "0x3333333333333333333333333333333333333333",
		AvatarURL:           "https://example.com/a.png", MFAEnrolled: true,
	}
	sub := &authv1.Subaccount{
		Id: 9, Role: authv1.SubaccountRole_OWNER, Label: "desk", Icon: "🚀", Color: "teal",
		Status:                       authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE,
		SmartAccountAddress:          "0x4444444444444444444444444444444444444444",
		OwnerUsername:                "alice",
		OwnerAvatarUrl:               "https://example.com/o.png",
		OwnerRootSmartAccountAddress: "0x5555555555555555555555555555555555555555",
		SubaccountPolicyId:           8, RequireMemberMfa: true, SmartAccountSaltNonce: proto.Uint32(3),
	}

	got := mustReply(GetSubaccountFromProto(&authv1.GetSubaccountResponse{
		Subaccount: sub, Members: []*authv1.SubaccountMemberView{member}, Invites: []*authv1.SubaccountInvite{invite},
	}))
	if !reflect.DeepEqual(got.Invites[0], wantInvite) {
		t.Errorf("Get invite = %+v, want %+v", got.Invites[0], wantInvite)
	}
	if got.Members[0] != wantMember {
		t.Errorf("Get member = %+v, want %+v", got.Members[0], wantMember)
	}
	s := got.Subaccount
	if s.SubaccountPolicyID != codecs.FormatID(8) || !s.RequireMemberMFA || s.SmartAccountSaltNonce == nil ||
		*s.SmartAccountSaltNonce != 3 || s.Icon != "🚀" || s.Color != "teal" || s.OwnerUsername != "alice" ||
		s.OwnerAvatarURL != "https://example.com/o.png" ||
		s.OwnerRootSmartAccountAddress != "0x5555555555555555555555555555555555555555" {
		t.Errorf("Get subaccount = %+v", *s)
	}

	invites := mustReply(SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{
		Invites: []*authv1.SubaccountInvite{invite},
	}))
	if !reflect.DeepEqual(invites.Invites[0], wantInvite) {
		t.Errorf("ListInvites = %+v, want %+v", invites.Invites[0], wantInvite)
	}
	members := mustReply(SubaccountMembersListFromProto(&authv1.ListSubaccountMembersResponse{
		Members: []*authv1.SubaccountMemberView{member},
	}))
	if members.Members[0] != wantMember {
		t.Errorf("ListMembers = %+v, want %+v", members.Members[0], wantMember)
	}
	list := mustReply(SubaccountsListFromProto(&authv1.ListSubaccountsResponse{Subaccounts: []*authv1.Subaccount{sub}}))
	if l := list.Subaccounts[0]; l.SubaccountPolicyID != codecs.FormatID(8) || !l.RequireMemberMFA {
		t.Errorf("List subaccount = %+v", l)
	}
}

// Fields the server leaves out stay absent: no inviter, no policy, no
// response time and no salt nonce read as "", nil and nil.
func TestSubaccountReadsLeaveUnsentFieldsAbsent(t *testing.T) {
	got := mustReply(GetSubaccountFromProto(&authv1.GetSubaccountResponse{
		Subaccount: &authv1.Subaccount{Id: 9},
		Invites:    []*authv1.SubaccountInvite{{Id: 4, SubaccountId: 2}},
	}))
	inv := got.Invites[0]
	if inv.InviterAccountID != "" || inv.CreatedAt != nil || inv.RespondedAt != nil || inv.RequireMemberMFA {
		t.Errorf("invite = %+v, want unsent fields absent", inv)
	}
	if s := got.Subaccount; s.SubaccountPolicyID != "" || s.SmartAccountSaltNonce != nil || s.RequireMemberMFA {
		t.Errorf("subaccount = %+v, want unsent fields absent", *s)
	}
}
