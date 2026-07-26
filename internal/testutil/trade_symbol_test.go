package testutil_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

func TestPickTradeSymbolHonorsTradeSymbolEnv(t *testing.T) {
	t.Setenv("POLYESTER_TEST_TRADE_SYMBOL", "BTC-USDT")
	t.Setenv("POLYESTER_TEST_SMOKE_SYMBOL", "ETH-USDT")
	t.Setenv("POLYESTER_SMOKE_SYMBOL", "ETH-USDT")

	spot := map[string]any{
		"pairs": []any{
			map[string]any{"symbol": "ETH-USDT"},
			map[string]any{"symbol": "BTC-USDT"},
		},
	}
	if got := testutil.PickTradeSymbol(spot); got != "BTC-USDT" {
		t.Fatalf("got %q want BTC-USDT (must not fall back to smoke ETH)", got)
	}
}

func TestPickTradeSymbolIgnoresSmokeEnvWhenTradeUnset(t *testing.T) {
	t.Setenv("POLYESTER_TEST_TRADE_SYMBOL", "")
	t.Setenv("POLYESTER_TEST_SMOKE_SYMBOL", "SOL-USDT")
	t.Setenv("POLYESTER_SMOKE_SYMBOL", "SOL-USDT")

	spot := map[string]any{
		"pairs": []any{
			map[string]any{"symbol": "ETH-USDT"},
			map[string]any{"symbol": "BTC-USDT"},
			map[string]any{"symbol": "SOL-USDT"},
		},
	}
	// Without TRADE_SYMBOL, picker uses candidate list — not smoke env.
	if got := testutil.PickTradeSymbol(spot); got != "ETH-USDT" {
		t.Fatalf("got %q want ETH-USDT from candidates (smoke env isolated)", got)
	}
}
