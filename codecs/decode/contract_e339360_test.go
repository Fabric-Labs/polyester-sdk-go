package decode

import (
	"reflect"
	"testing"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	"google.golang.org/protobuf/proto"
)

// The seven account permissions that open deposit-address, sub-account and
// Guard Signer calls to API keys read back by their contract names, and each
// name maps back to its value. CREATE_DEPOSIT_ADDRESS keeps its earlier value
// 10, among the product actions: classify by name, not by number range.
func TestAPIPolicyKeepsDepositSubaccountAndGuardSignerPermissions(t *testing.T) {
	want := map[string]int32{
		"CREATE_DEPOSIT_ADDRESS":   10,
		"READ_DEPOSIT_ADDRESSES":   int32(authv1.PolicyAction_READ_DEPOSIT_ADDRESSES),
		"READ_SUBACCOUNTS":         int32(authv1.PolicyAction_READ_SUBACCOUNTS),
		"READ_SUBACCOUNT_ACTIVITY": int32(authv1.PolicyAction_READ_SUBACCOUNT_ACTIVITY),
		"READ_SUBACCOUNT_MEMBERS":  int32(authv1.PolicyAction_READ_SUBACCOUNT_MEMBERS),
		"READ_SUBACCOUNT_INVITES":  int32(authv1.PolicyAction_READ_SUBACCOUNT_INVITES),
		"READ_GUARD_SIGNER_STATUS": int32(authv1.PolicyAction_READ_GUARD_SIGNER_STATUS),
	}
	names := []string{"READ_ADDRESS_BOOK", "CREATE_DEPOSIT_ADDRESS", "READ_DEPOSIT_ADDRESSES", "READ_SUBACCOUNTS",
		"READ_SUBACCOUNT_ACTIVITY", "READ_SUBACCOUNT_MEMBERS", "READ_SUBACCOUNT_INVITES", "READ_GUARD_SIGNER_STATUS"}
	actions := make([]authv1.PolicyAction, 0, len(names))
	for _, name := range names {
		value, ok := authv1.PolicyAction_value[name]
		if !ok {
			t.Fatalf("PolicyAction %s is not in the contract", name)
		}
		if expected, pinned := want[name]; pinned && value != expected {
			t.Fatalf("%s = %d, want %d", name, value, expected)
		}
		actions = append(actions, authv1.PolicyAction(value))
	}
	payload, err := proto.Marshal(&authv1.ApiPolicyView{Id: 4, Actions: actions})
	if err != nil {
		t.Fatal(err)
	}
	var wire authv1.ApiPolicyView
	if err := proto.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if got := ApiPolicyFromView(&wire, nil).Actions; !reflect.DeepEqual(got, names) {
		t.Fatalf("actions = %v, want %v", got, names)
	}
}

// API keys report their purpose: the user's own keys read STANDARD and the
// assistant's keys ASSISTANT.
func TestAPIKeyReadsPurpose(t *testing.T) {
	cases := map[authv1.ApiKeyPurpose]string{
		authv1.ApiKeyPurpose_STANDARD:            "STANDARD",
		authv1.ApiKeyPurpose_ASSISTANT:           "ASSISTANT",
		authv1.ApiKeyPurpose_PURPOSE_UNSPECIFIED: "PURPOSE_UNSPECIFIED",
	}
	for purpose, want := range cases {
		got := ApiKeyMessageFromProto(&authv1.ApiKey{KeyId: "ak_1", Purpose: purpose})
		if got.Purpose != want {
			t.Fatalf("purpose %v reads %q, want %q", purpose, got.Purpose, want)
		}
	}
}

// A withdrawal Zipper transferred for compliance names its reason instead of
// an unknown code.
func TestLifecycleReasonNamesComplianceTransfer(t *testing.T) {
	if got := lifecycleReasonLabel(lifecyclev1.LifecycleReason_ZIPPER_COMPLIANCE_TRANSFERRED); got != "zipper_compliance_transferred" {
		t.Fatalf("label = %q", got)
	}
	flow := FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{LifecycleReason: lifecyclev1.LifecycleReason_ZIPPER_COMPLIANCE_TRANSFERRED})
	if flow.LifecycleReason != "zipper_compliance_transferred" {
		t.Fatalf("flow reason = %q", flow.LifecycleReason)
	}
}

// included_in_equity keeps its presence: absent is unknown (nil), never false.
func TestSpotAssetIncludedInEquityKeepsPresence(t *testing.T) {
	yes, no := true, false
	got, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{Assets: []*marketdatav1.AssetConfig{
		{Asset: "BTC", LedgerId: 3, IncludedInEquity: &yes},
		{Asset: "PTS", LedgerId: 9, IncludedInEquity: &no},
		{Asset: "OLD", LedgerId: 11},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 3 {
		t.Fatalf("assets = %+v", got.Assets)
	}
	if v := got.Assets[0].IncludedInEquity; v == nil || !*v {
		t.Fatalf("BTC included_in_equity = %v, want true", v)
	}
	if v := got.Assets[1].IncludedInEquity; v == nil || *v {
		t.Fatalf("PTS included_in_equity = %v, want false", v)
	}
	if v := got.Assets[2].IncludedInEquity; v != nil {
		t.Fatalf("absent included_in_equity = %v, want nil (unknown)", *v)
	}
}

// Rate-limit refusals of the assistant name their policy class.
func TestRateLimitDetailNamesAssistantPolicyClasses(t *testing.T) {
	for _, name := range []string{"ASSISTANT_CONTROL", "ASSISTANT_MESSAGE", "ASSISTANT_READ"} {
		value, ok := ratelimitv1.PolicyClass_value[name]
		if !ok {
			t.Fatalf("PolicyClass %s is not in the contract", name)
		}
		got := RateLimitDetailFromProto(&ratelimitv1.RateLimitDetail{PolicyClass: ratelimitv1.PolicyClass(value)})
		if got.PolicyClass != name {
			t.Fatalf("policy class %d reads %q, want %q", value, got.PolicyClass, name)
		}
	}
}

// The debit of a withdrawal Zipper rejected reads transfer code 1075, a debit
// with no flow ID.
func TestTransferRowReadsWithdrawFailedDebit(t *testing.T) {
	got := TransferRowFromProto(&ledgerrdv1.TransferRow{
		TransferId:   "t-1",
		AssetId:      7,
		TransferCode: ledgerv1.TransferCode_WITHDRAW_FAILED,
		IsDebit:      true,
	})
	if got.TransferType != 1075 || !got.IsDebit || got.TxID != "" {
		t.Fatalf("transfer = %+v", got)
	}
}
