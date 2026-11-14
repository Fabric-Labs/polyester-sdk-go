package decode_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Each chain variant says whether its route accepts new deposits and new
// withdrawals; an unspecified status is not known, so not available.
func TestDepositWithdrawConfigRouteStatuses(t *testing.T) {
	cfg := decode.DepositWithdrawConfigFromProto(&zipperv1.GetDepositWithdrawConfigResponse{
		Chains: []*zipperv1.ChainConfig{{ChainId: 2, Code: "ethereum"}},
		Assets: []*zipperv1.AssetConfig{{
			Asset: "USDT", LedgerId: 3, QuantityScale: 6,
			Variants: []*zipperv1.AssetChainVariant{
				{ZippedAssetId: 3, ChainId: 2, DepositStatus: zipperv1.RouteStatus_NORMAL, WithdrawalStatus: zipperv1.RouteStatus_SUSPENDED},
				{ZippedAssetId: 4, ChainId: 2},
				{ZippedAssetId: 5, ChainId: 2, DepositStatus: zipperv1.RouteStatus(7), WithdrawalStatus: zipperv1.RouteStatus_NORMAL},
			},
		}},
	})
	variants := cfg.Assets[0].Variants
	type status struct {
		deposit, withdrawal string
		accepts, sends      bool
	}
	want := []status{
		{models.RouteStatusNormal, models.RouteStatusSuspended, true, false},
		{"", "", false, false},
		{"unknown(7)", models.RouteStatusNormal, false, true},
	}
	for i, w := range want {
		v := variants[i]
		got := status{v.DepositStatus, v.WithdrawalStatus, v.AcceptsDeposits(), v.AcceptsWithdrawals()}
		if got != w {
			t.Fatalf("variant %d = %+v, want %+v", v.ZippedAssetID, got, w)
		}
	}

	// The zipper catalog carries the statuses too.
	chains := catalogs.BuildZipperCatalogData(&cfg).Assets[0].Chains
	if len(chains) != 3 || chains[0].DepositStatus != models.RouteStatusNormal || chains[0].WithdrawalStatus != models.RouteStatusSuspended ||
		!chains[0].AcceptsDeposits() || chains[0].AcceptsWithdrawals() || chains[1].AcceptsDeposits() {
		t.Fatalf("catalog chains = %+v", chains)
	}

	encoded, err := json.Marshal(variants[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["deposit_status"] != "normal" || fields["withdrawal_status"] != "suspended" {
		t.Fatalf("JSON = %s", encoded)
	}
}

// Policy reads name the statement-export permission.
func TestPolicyActionsNameReadStatements(t *testing.T) {
	actions := []authv1.PolicyAction{authv1.PolicyAction_READ_BALANCES, authv1.PolicyAction_READ_STATEMENTS}
	sub := decode.SubaccountPolicyMessageFromProto(&authv1.SubaccountPolicyView{Id: 1, Actions: actions})
	api := decode.ApiPolicyMessageFromProto(&authv1.ApiPolicyView{Id: 2, Actions: actions})
	want := []string{"READ_BALANCES", "READ_STATEMENTS"}
	if !reflect.DeepEqual(sub.Actions, want) || !reflect.DeepEqual(api.Actions, want) {
		t.Fatalf("actions = %v and %v, want %v", sub.Actions, api.Actions, want)
	}
	if authv1.PolicyAction_READ_STATEMENTS != 38 || authv1.SubaccountPermission_SUBACCOUNT_PERMISSION_READ_STATEMENTS != 29 {
		t.Fatal("READ_STATEMENTS numbers moved")
	}
}

// Get, list and stream decode one complete API key: the attached policy, the
// public key, the creator and a fractional creation time are kept.
func TestApiKeyReadsKeepTheCompleteModel(t *testing.T) {
	created := time.Date(2026, 11, 12, 16, 48, 15, 912958000, time.UTC)
	policy := uint64(77)
	key := &authv1.ApiKey{
		KeyId: "ak_0123456789abcdef0123456789abcdef", Label: "desk", CreatedByActor: "alice",
		PublicKeyEd25519: []byte{0xde, 0xad, 0xbe, 0xef}, PolicyId: &policy, Revision: 5,
		CreatedAt: timestamppb.New(created),
	}
	payload, err := proto.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := decode.ApiKeyFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	got := []models.ApiKeySummary{
		*decode.ApiKeyFromGetProto(&authv1.GetApiKeyResponse{ApiKey: key}),
		decode.ApiKeysListFromProto(&authv1.ListApiKeysResponse{ApiKeys: []*authv1.ApiKey{key}}).Keys[0],
		streamed,
	}
	for i, k := range got {
		if k.PolicyID != codecs.FormatID(77) || k.PublicKeyEd25519 != "deadbeef" || k.CreatedByActor != "alice" ||
			k.CreatedAt == nil || !k.CreatedAt.Equal(created) {
			t.Fatalf("read %d = %+v", i, k)
		}
	}
}
