package decode

import (
	"testing"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// One role reads the same on the sub-account, its members and its invites.
func TestSubaccountRoleSpellingMatchesAcrossSubaccountMembersAndInvites(t *testing.T) {
	for role, want := range map[authv1.SubaccountRole]string{
		authv1.SubaccountRole_OWNER:            "owner",
		authv1.SubaccountRole_LEVERAGED_TRADER: "leveraged_trader",
		authv1.SubaccountRole_VIEWER:           "viewer",
	} {
		got := GetSubaccountFromProto(&authv1.GetSubaccountResponse{
			Subaccount: &authv1.Subaccount{Id: 9, Role: role},
			Members:    []*authv1.SubaccountMemberView{{AccountId: 3, Role: role}},
			Invites:    []*authv1.SubaccountInvite{{Id: 4, GranteeAccountId: 5, Role: role}},
		})
		if got.Subaccount.Role != want || got.Members[0].Role != want || got.Invites[0].Role != want {
			t.Errorf("%v: subaccount=%q member=%q invite=%q, want %q everywhere",
				role, got.Subaccount.Role, got.Members[0].Role, got.Invites[0].Role, want)
		}
		members := SubaccountMembersListFromProto(&authv1.ListSubaccountMembersResponse{
			Members: []*authv1.SubaccountMemberView{{AccountId: 3, Role: role}},
		})
		invites := SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{
			Invites: []*authv1.SubaccountInvite{{Id: 4, Role: role}},
		})
		if members.Members[0].Role != want || invites.Invites[0].Role != want {
			t.Errorf("%v: ListMembers=%q ListInvites=%q, want %q", role, members.Members[0].Role, invites.Invites[0].Role, want)
		}
	}
	// An unset role reads "" everywhere, never a default role.
	got := GetSubaccountFromProto(&authv1.GetSubaccountResponse{
		Subaccount: &authv1.Subaccount{Id: 9},
		Members:    []*authv1.SubaccountMemberView{{AccountId: 3}},
		Invites:    []*authv1.SubaccountInvite{{Id: 4}},
	})
	if got.Subaccount.Role != "" || got.Members[0].Role != "" || got.Invites[0].Role != "" {
		t.Errorf("unset role: subaccount=%q member=%q invite=%q, want empty", got.Subaccount.Role, got.Members[0].Role, got.Invites[0].Role)
	}
}
