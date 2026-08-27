package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestSubaccountPolicyFromViewUsesSymbolID(t *testing.T) {
	msg := &authv1.SubaccountPolicyView{
		Id:              7,
		Name:            "trader",
		Revision:        3,
		SpotMarketScope: authv1.MarketScope_ALLOWLIST,
		Actions: []authv1.PolicyAction{
			authv1.PolicyAction_TRADE_SPOT,
			authv1.PolicyAction_READ_ADDRESS_BOOK,
		},
		SpotMarkets: []*authv1.SpotMarketRule{
			{SymbolId: 11},
		},
		MaxOrderNotional: 1_000_000,
		MaxOpenOrders:    5,
		TradingHalted:    true,
	}
	policy := decode.SubaccountPolicyFromView(msg, nil)
	if policy == nil {
		t.Fatal("expected policy")
	}
	if policy.PolicyID != codecs.FormatUint64ID(7) || policy.SpotMarketScope != "ALLOWLIST" {
		t.Fatalf("policy=%+v", policy)
	}
	if len(policy.SpotMarkets) != 1 || policy.SpotMarkets[0].SymbolID != 11 || policy.SpotMarkets[0].Symbol != "" {
		t.Fatalf("spot_markets=%+v", policy.SpotMarkets)
	}
	if len(policy.Actions) != 2 || policy.Actions[0] != "TRADE_SPOT" || policy.Actions[1] != "READ_ADDRESS_BOOK" {
		t.Fatalf("actions=%v", policy.Actions)
	}
	if !policy.TradingHalted || policy.MaxOpenOrders != 5 {
		t.Fatalf("limits=%+v", policy)
	}
}
