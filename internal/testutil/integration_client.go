package testutil

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
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
// SmokeSymbol resolves a liquid devnet pair symbol from spot config.
func SmokeSymbol(t *testing.T, client *polyester.Client, ctx context.Context) string {
	t.Helper()
	cfg := CallRequired(t, "market_data.get_spot_config", func() (models.SpotConfig, error) {
		return client.MarketData.GetSpotConfig(ctx)
	})
	if client.Catalogs != nil {
		client.Catalogs.HydrateSpotConfig(cfg.Raw)
	}
	return PickSmokeSymbol(cfg.Raw)
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
