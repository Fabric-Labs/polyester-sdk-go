package decode_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// The create receipt names the lifecycle flow, so a caller can read the
// transfer's flow without matching amounts or timestamps.
func TestInternalTransferExposesFlowID(t *testing.T) {
	got, err := decode.InternalTransferFromProto(&transferv1.CreateInternalTransferResponse{
		RequestId:  "11",
		TransferId: "254811058965469215576143367742519946794",
		AssetId:    2,
		FlowId:     "flow-7",
		Status:     transferv1.InternalTransferStatus_INTERNAL_TRANSFER_STATUS_ACCEPTED,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.FlowID != "flow-7" {
		t.Fatalf("FlowID=%q", got.FlowID)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["flow_id"] != "flow-7" {
		t.Fatalf("json=%s", encoded)
	}

	// An older server sends no flow ID: the receipt still decodes, flow_id
	// stays empty and is left out of JSON.
	old, err := decode.InternalTransferFromProto(&transferv1.CreateInternalTransferResponse{RequestId: "11", TransferId: "12"})
	if err != nil || old.FlowID != "" {
		t.Fatalf("old=%#v err=%v", old, err)
	}
	encoded, _ = json.Marshal(old)
	fields = nil
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["flow_id"]; ok {
		t.Fatalf("json=%s", encoded)
	}
}

// Chain config carries the memo rule, label and format hint of each network.
// The hint is display text and is kept verbatim.
func TestDepositWithdrawConfigCarriesMemoRule(t *testing.T) {
	hint := "Numeric, 0 to 4294967295; leave empty unless the exchange asks for one."
	cfg := decode.DepositWithdrawConfigFromProto(&zipperv1.GetDepositWithdrawConfigResponse{Chains: []*zipperv1.ChainConfig{
		{ChainId: 1, Code: "ethereum", MemoRequirement: zipperv1.MemoRequirement_NOT_SUPPORTED},
		{ChainId: 7, Code: "xrp", MemoRequirement: zipperv1.MemoRequirement_OPTIONAL, MemoLabel: "Destination tag", MemoFormatHint: hint},
		{ChainId: 8, Code: "stellar", MemoRequirement: zipperv1.MemoRequirement_REQUIRED, MemoLabel: "Memo"},
		{ChainId: 9, Code: "older"},
		{ChainId: 10, Code: "future", MemoRequirement: zipperv1.MemoRequirement(9)},
	}})
	want := []struct {
		requirement, label, hint string
	}{
		{models.MemoRequirementNotSupported, "", ""},
		{models.MemoRequirementOptional, "Destination tag", hint},
		{models.MemoRequirementRequired, "Memo", ""},
		{"", "", ""},
		{"UNKNOWN(9)", "", ""},
	}
	if len(cfg.Chains) != len(want) {
		t.Fatalf("chains=%#v", cfg.Chains)
	}
	for i, w := range want {
		got := cfg.Chains[i]
		if got.MemoRequirement != w.requirement || got.MemoLabel != w.label || got.MemoFormatHint != w.hint {
			t.Fatalf("chain %d: requirement=%q label=%q hint=%q", got.ChainID, got.MemoRequirement, got.MemoLabel, got.MemoFormatHint)
		}
	}
	if models.MemoRequirementRequired != "required" || models.MemoRequirementOptional != "optional" ||
		models.MemoRequirementNotSupported != "not_supported" {
		t.Fatal("memo requirement labels changed")
	}
	encoded, err := json.Marshal(cfg.Chains[1])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["memo_requirement"] != "optional" || fields["memo_label"] != "Destination tag" || fields["memo_format_hint"] != hint {
		t.Fatalf("json=%s", encoded)
	}
}

// accountPermissions are the PolicyAction values added for API-key policies:
// READ_API_KEYS and every value after it.
var accountPermissions = []authv1.PolicyAction{
	authv1.PolicyAction_READ_API_KEYS,
	authv1.PolicyAction_READ_API_POLICIES,
	authv1.PolicyAction_READ_SUBACCOUNT_POLICIES,
	authv1.PolicyAction_READ_SUBACCOUNT_PERMISSIONS,
	authv1.PolicyAction_READ_VIP_STATUS,
	authv1.PolicyAction_READ_SOCIAL_VERIFICATION,
	authv1.PolicyAction_READ_PROFILE,
	authv1.PolicyAction_READ_ACCOUNT_DIRECTORY,
	authv1.PolicyAction_READ_WITHDRAW_VALIDATION,
	authv1.PolicyAction_READ_DAILY_CLAIMS,
	authv1.PolicyAction_CLAIM_DAILY_REWARD,
	authv1.PolicyAction_READ_CHECKPOINTS,
	authv1.PolicyAction_READ_REWARDS,
}

// An API-key policy that grants the account permissions reads back every one
// by its contract name, and each name maps back to the same value.
func TestAPIPolicyKeepsAccountPermissions(t *testing.T) {
	actions := append([]authv1.PolicyAction{authv1.PolicyAction_READ_SPOT}, accountPermissions...)
	payload, err := proto.Marshal(&authv1.ApiPolicyView{Id: 3, Actions: actions})
	if err != nil {
		t.Fatal(err)
	}
	var wire authv1.ApiPolicyView
	if err := proto.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	got := decode.ApiPolicyFromView(&wire, nil).Actions
	want := []string{
		"READ_SPOT", "READ_API_KEYS", "READ_API_POLICIES", "READ_SUBACCOUNT_POLICIES",
		"READ_SUBACCOUNT_PERMISSIONS", "READ_VIP_STATUS", "READ_SOCIAL_VERIFICATION",
		"READ_PROFILE", "READ_ACCOUNT_DIRECTORY", "READ_WITHDRAW_VALIDATION",
		"READ_DAILY_CLAIMS", "CLAIM_DAILY_REWARD", "READ_CHECKPOINTS", "READ_REWARDS",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions=%v", got)
	}
	for i, name := range want {
		value, ok := authv1.PolicyAction_value[name]
		if !ok || authv1.PolicyAction(value) != actions[i] {
			t.Fatalf("%s maps to %d (known=%v), want %d", name, value, ok, actions[i])
		}
	}
	if authv1.PolicyAction_READ_API_KEYS != 17 {
		t.Fatalf("READ_API_KEYS=%d", authv1.PolicyAction_READ_API_KEYS)
	}
}

// Code 1074 is a balance adjustment with a system account, such as account
// seeding. Its direction is IsDebit; the code says neither credit nor debit.
func TestSystemAdjustmentDirectionComesFromIsDebit(t *testing.T) {
	seeding := &ledgerrdv1.TransferRow{
		TransferId:   "41",
		AssetId:      2,
		AmountE18:    &typev1.U128{Lo: 5_000_000_000_000_000_000},
		TransferCode: ledgerv1.TransferCode_SYSTEM_ADJUSTMENT,
		IsDebit:      false,
		Source:       &ledgerrdv1.TransferSide{Kind: ledgerrdv1.TransferSideKind_SYSTEM_ACCOUNT},
		Destination:  &ledgerrdv1.TransferSide{Kind: ledgerrdv1.TransferSideKind_TRADING_ACCOUNT},
	}
	correction := &ledgerrdv1.TransferRow{
		TransferId:   "42",
		AssetId:      2,
		AmountE18:    &typev1.U128{Lo: 1_000_000_000_000_000_000},
		TransferCode: ledgerv1.TransferCode_SYSTEM_ADJUSTMENT,
		IsDebit:      true,
		Source:       &ledgerrdv1.TransferSide{Kind: ledgerrdv1.TransferSideKind_TRADING_ACCOUNT},
		Destination:  &ledgerrdv1.TransferSide{Kind: ledgerrdv1.TransferSideKind_SYSTEM_ACCOUNT},
	}
	payload, err := proto.Marshal(&ledgerrdv1.TransferRowBatch{Events: []*ledgerrdv1.TransferRow{seeding, correction}})
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := decode.LedgerTransferBatchFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	listed := decode.TransfersListFromProto(&ledgerrdv1.ListTransfersResponse{Transfers: []*ledgerrdv1.TransferRow{seeding, correction}}).Transfers
	for name, rows := range map[string][]models.LedgerTransfer{"stream": streamed, "list": listed} {
		if len(rows) != 2 {
			t.Fatalf("%s: rows=%#v", name, rows)
		}
		credit, debit := rows[0], rows[1]
		if credit.TransferType != 1074 || credit.IsDebit || credit.Amount != "5000000000000000000" ||
			credit.Source == nil || credit.Source.Kind != "system_account" {
			t.Fatalf("%s: seeding credit=%#v", name, credit)
		}
		if debit.TransferType != 1074 || !debit.IsDebit || debit.Amount != "1000000000000000000" ||
			debit.Destination == nil || debit.Destination.Kind != "system_account" {
			t.Fatalf("%s: debit correction=%#v", name, debit)
		}
	}
}

// The transfer codes this contract names keep their numeric code on every
// transfer read.
func TestNamedTransferCodesKeepTheirCode(t *testing.T) {
	codes := map[ledgerv1.TransferCode]int{
		ledgerv1.TransferCode_TRADING_WITHDRAW_FEE_LEG: 1066,
		ledgerv1.TransferCode_QUICKSWAP_POLYESTER_FEE:  1067,
		ledgerv1.TransferCode_QUICKSWAP_BROKER_FEE:     1068,
		ledgerv1.TransferCode_QUICKSWAP_AFFILIATE_FEE:  1069,
		ledgerv1.TransferCode_FEE_SWEEP:                1072,
		ledgerv1.TransferCode_FEE_REFILL:               1073,
		ledgerv1.TransferCode_SYSTEM_ADJUSTMENT:        1074,
	}
	for code, want := range codes {
		row := decode.TransferRowFromProto(&ledgerrdv1.TransferRow{TransferId: "1", TransferCode: code, IsDebit: true})
		if row.TransferType != want || !row.IsDebit {
			t.Fatalf("%s: row=%#v", code, row)
		}
	}
}
