package codecs_test

import (
	"testing"
	"time"

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

func TestPolicyCodecsNestedCreate(t *testing.T) {
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
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if req.GetPolicy() == nil || req.GetPolicy().GetName() != "test" {
		t.Fatalf("req=%+v", req)
	}
	if len(req.GetPolicy().GetActions()) != 1 || req.GetPolicy().GetActions()[0] != authv1.PolicyAction_READ_BALANCES {
		t.Fatalf("actions=%v", req.GetPolicy().GetActions())
	}
}

func TestUpdateSubaccountOneFieldPatch(t *testing.T) {
	label := "renamed"
	req, err := codecs.BuildUpdateSubaccountRequest(42, codecs.SubaccountPatch{
		ExpectedRevision: 7,
		Label:            &label,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if req.GetSubaccountId() != 42 || req.GetExpectedRevision() != 7 {
		t.Fatalf("ids=%+v", req)
	}
	if got := req.GetUpdateMask().GetPaths(); len(got) != 1 || got[0] != "label" {
		t.Fatalf("mask=%v", got)
	}
	if req.GetSubaccount().GetLabel() != "renamed" {
		t.Fatalf("spec=%+v", req.GetSubaccount())
	}
}

func TestUpdateSubaccountEmptyStringPresence(t *testing.T) {
	empty := ""
	req, err := codecs.BuildUpdateSubaccountRequest(1, codecs.SubaccountPatch{
		ExpectedRevision: 1,
		Label:            &empty,
		Icon:             &empty,
		Color:            &empty,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) != 3 {
		t.Fatalf("paths=%v", paths)
	}
	if req.GetSubaccount().GetLabel() != "" || req.GetSubaccount().GetIcon() != "" || req.GetSubaccount().GetColor() != "" {
		t.Fatalf("spec=%+v", req.GetSubaccount())
	}
}

func TestUpdateApiKeyEmptySliceAndFalsePresence(t *testing.T) {
	emptyWhitelist := []string{}
	status := "disabled"
	req, err := codecs.BuildUpdateApiKeyRequest("ak_0123456789abcdef0123456789abcdef", codecs.ApiKeyPatch{
		ExpectedRevision: 3,
		Status:           &status,
		IpWhitelist:      &emptyWhitelist,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) != 2 || paths[0] != "status" || paths[1] != "ip_whitelist" {
		t.Fatalf("paths=%v", paths)
	}
	if req.GetApiKey().GetStatus() != authv1.ApiKeyStatus_DISABLED {
		t.Fatalf("status=%v", req.GetApiKey().GetStatus())
	}
	if req.GetApiKey().GetIpWhitelist() == nil || len(req.GetApiKey().GetIpWhitelist()) != 0 {
		t.Fatalf("whitelist=%v", req.GetApiKey().GetIpWhitelist())
	}
}

func TestUpdateApiKeyTimestampClear(t *testing.T) {
	req, err := codecs.BuildUpdateApiKeyRequest("ak_0123456789abcdef0123456789abcdef", codecs.ApiKeyPatch{
		ExpectedRevision: 2,
		ExpiresAt:        codecs.TimeClear(),
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := req.GetUpdateMask().GetPaths(); len(got) != 1 || got[0] != "expires_at" {
		t.Fatalf("mask=%v", got)
	}
	if req.GetApiKey().GetExpiresAt() != nil {
		t.Fatalf("expected nil expires_at clear, got %v", req.GetApiKey().GetExpiresAt())
	}

	ts := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	req, err = codecs.BuildUpdateApiKeyRequest("ak_0123456789abcdef0123456789abcdef", codecs.ApiKeyPatch{
		ExpectedRevision: 2,
		ExpiresAt:        codecs.TimeSet(ts),
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !req.GetApiKey().GetExpiresAt().AsTime().Equal(ts) {
		t.Fatalf("expires_at=%v", req.GetApiKey().GetExpiresAt())
	}
}

func TestUpdateSubaccountPolicyZeroAndFalsePresence(t *testing.T) {
	zero := uint64(0)
	halted := false
	emptyActions := []string{}
	req, err := codecs.BuildUpdateSubaccountPolicyRequest(9, codecs.SubaccountPolicyPatch{
		ExpectedRevision: 4,
		MaxOrderNotional: &zero,
		TradingHalted:    &halted,
		Actions:          &emptyActions,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) != 3 {
		t.Fatalf("paths=%v", paths)
	}
	if req.GetPolicy().GetMaxOrderNotional() != 0 {
		t.Fatalf("max_order_notional=%d", req.GetPolicy().GetMaxOrderNotional())
	}
	if req.GetPolicy().GetTradingHalted() {
		t.Fatal("expected trading_halted=false")
	}
	if req.GetPolicy().GetActions() != nil && len(req.GetPolicy().GetActions()) != 0 {
		t.Fatalf("actions=%v", req.GetPolicy().GetActions())
	}
}

func TestUpdateSubaccountPolicyTimestampClear(t *testing.T) {
	req, err := codecs.BuildUpdateSubaccountPolicyRequest(1, codecs.SubaccountPolicyPatch{
		ExpectedRevision: 1,
		ReviewAt:         codecs.TimeClear(),
		ExpiresAt:        codecs.TimeClear(),
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) != 2 || paths[0] != "review_at" || paths[1] != "expires_at" {
		t.Fatalf("paths=%v", paths)
	}
	if req.GetPolicy().GetReviewAt() != nil || req.GetPolicy().GetExpiresAt() != nil {
		t.Fatalf("expected cleared timestamps, got review=%v expires=%v", req.GetPolicy().GetReviewAt(), req.GetPolicy().GetExpiresAt())
	}
}

func TestUpdateRequiresPositiveRevisionAndNonEmptyMask(t *testing.T) {
	label := "x"
	if _, err := codecs.BuildUpdateSubaccountRequest(1, codecs.SubaccountPatch{Label: &label}); err == nil {
		t.Fatal("expected revision validation error")
	}
	if _, err := codecs.BuildUpdateSubaccountRequest(1, codecs.SubaccountPatch{ExpectedRevision: 1}); err == nil {
		t.Fatal("expected empty mask validation error")
	}
}

func TestUpdateAddressBookEntryEmptyTagIDs(t *testing.T) {
	empty := []string{}
	note := ""
	req, err := codecs.BuildUpdateAddressBookEntryRequest(11, codecs.AddressBookEntryPatch{
		ExpectedRevision: 5,
		Note:             &note,
		TagIDs:           &empty,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) != 2 || paths[0] != "note" || paths[1] != "tag_ids" {
		t.Fatalf("paths=%v", paths)
	}
	if req.GetEntry().GetNote() != "" {
		t.Fatalf("note=%q", req.GetEntry().GetNote())
	}
	if req.GetEntry().GetTagIds() == nil || len(req.GetEntry().GetTagIds()) != 0 {
		t.Fatalf("tag_ids=%v", req.GetEntry().GetTagIds())
	}
}
