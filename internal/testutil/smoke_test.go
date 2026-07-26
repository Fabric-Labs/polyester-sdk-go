package testutil_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

func TestPickSmokeSymbolPrefersCandidates(t *testing.T) {
	t.Setenv("POLYESTER_TEST_TRADE_SYMBOL", "")
	spot := map[string]any{
		"pairs": []any{
			map[string]any{"symbol": "SOL-USDT"},
			map[string]any{"symbol": "ETH-USDT"},
		},
	}
	if got := testutil.PickSmokeSymbol(spot); got != "ETH-USDT" {
		t.Fatalf("got %q want ETH-USDT", got)
	}
}

func TestPickSmokeSymbolFallsBackToFirstPair(t *testing.T) {
	t.Setenv("POLYESTER_TEST_TRADE_SYMBOL", "")
	spot := map[string]any{
		"pairs": []any{map[string]any{"symbol": "FOO-BAR"}},
	}
	if got := testutil.PickSmokeSymbol(spot); got != "FOO-BAR" {
		t.Fatalf("got %q want FOO-BAR", got)
	}
}

func TestPickSmokeSymbolDefaultWhenEmpty(t *testing.T) {
	t.Setenv("POLYESTER_TEST_TRADE_SYMBOL", "")
	if got := testutil.PickSmokeSymbol(nil); got != "ETH-USDT" {
		t.Fatalf("got %q want ETH-USDT", got)
	}
}
