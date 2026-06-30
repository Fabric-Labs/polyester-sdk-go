package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestSubaccountRoleFromLabel(t *testing.T) {
	role, err := codecs.SubaccountRoleFromLabel("trader")
	if err != nil || role != authv1.SubaccountRole_TRADER {
		t.Fatalf("trader: role=%v err=%v", role, err)
	}
	role, err = codecs.SubaccountRoleFromLabel("SUBACCOUNT_ROLE_ADMIN")
	if err != nil || role != authv1.SubaccountRole_ADMIN {
		t.Fatalf("admin: role=%v err=%v", role, err)
	}
}

func TestSubaccountInviteActionFromLabel(t *testing.T) {
	for _, action := range []string{"accept", "decline", "cancel"} {
		if _, err := codecs.SubaccountInviteActionFromLabel(action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if _, err := codecs.SubaccountInviteActionFromLabel("invalid"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestApiKeyStatusFromLabel(t *testing.T) {
	status, err := codecs.ApiKeyStatusFromLabel("active")
	if err != nil || status != authv1.ApiKeyStatus_ACTIVE {
		t.Fatalf("active: status=%v err=%v", status, err)
	}
}

func TestPolicyCodecs(t *testing.T) {
	scope, err := codecs.MarketScopeFromLabel("allowlist")
	if err != nil || scope != authv1.MarketScope_ALLOWLIST {
		t.Fatalf("scope=%v err=%v", scope, err)
	}
	action, err := codecs.PolicyActionFromLabel("trade_spot")
	if err != nil || action != authv1.PolicyAction_TRADE_SPOT {
		t.Fatalf("action=%v err=%v", action, err)
	}
	req, err := codecs.BuildCreateSubaccountPolicyRequest(nil, codecs.SubaccountPolicyWriteOpts{
		Name: "test", SpotMarketScope: "all", PerpMarketScope: "all",
		Actions: []string{"read_balances"},
	})
	if err != nil || req.GetName() != "test" {
		t.Fatalf("req=%+v err=%v", req, err)
	}
}
