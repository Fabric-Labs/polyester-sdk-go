package testutil

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
)

// RequireLiveClient returns a devnet client and timed context, or skips when credentials are missing.
func RequireLiveClient(t *testing.T) (*polyester.Client, context.Context, func()) {
	t.Helper()
	client, ok, err := LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		SoftSkip(t, "POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cleanup := func() {
		cancel()
		_ = client.Close()
	}
	return client, ctx, cleanup
}

// SmokeSymbol resolves the live-test trade symbol (POLYESTER_TEST_TRADE_SYMBOL).
// Kept as a compatibility alias for TradeSymbol; smoke-env fallbacks are not used.
func SmokeSymbol(t *testing.T, client *polyester.Client, ctx context.Context) string {
	t.Helper()
	return TradeSymbol(t, client, ctx)
}

// NonNegativeIntString parses a non-negative integer string field (including u128-sized values).
func NonNegativeIntString(t *testing.T, value string) *big.Int {
	t.Helper()
	if value == "" {
		return big.NewInt(0)
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("parse %q: invalid integer", value)
	}
	if n.Sign() < 0 {
		t.Fatalf("expected non-negative value, got %s", value)
	}
	return n
}

// NonNegativeIntStringPositive is like NonNegativeIntString but requires value > 0.
func NonNegativeIntStringPositive(t *testing.T, value string) *big.Int {
	t.Helper()
	n := NonNegativeIntString(t, value)
	if n.Sign() == 0 {
		t.Fatalf("expected positive value, got %s", value)
	}
	return n
}
