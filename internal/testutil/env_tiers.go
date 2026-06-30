package testutil

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go"
)

// RequireMutation skips unless POLYESTER_TEST_MUTATION is truthy.
func RequireMutation(t *testing.T) {
	t.Helper()
	if !EnvTruthy("POLYESTER_TEST_MUTATION") {
		t.Skip("Set POLYESTER_TEST_MUTATION=1 to run mutation tests")
	}
}

// RequireFunded skips unless POLYESTER_TEST_FUNDED is truthy.
func RequireFunded(t *testing.T) {
	t.Helper()
	if !EnvTruthy("POLYESTER_TEST_FUNDED") {
		t.Skip("Set POLYESTER_TEST_FUNDED=1 to run funded tests")
	}
}

// TradeE2EEnabled reports whether spot-fill e2e tests are enabled.
func TradeE2EEnabled() bool {
	return EnvTruthy("POLYESTER_TEST_TRADE_E2E")
}

// InternalTransferDest returns the configured internal transfer destination account id.
func InternalTransferDest() string {
	return strings.TrimSpace(os.Getenv("POLYESTER_TEST_INTERNAL_TRANSFER_DEST"))
}

// EnvTradeSymbol returns an explicit trade symbol override when set.
func EnvTradeSymbol() string {
	return strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRADE_SYMBOL"))
}

// PickTradeSymbol chooses the trade symbol for mutation/funded tests.
func PickTradeSymbol(spotRaw map[string]any) string {
	if override := EnvTradeSymbol(); override != "" {
		return override
	}
	return PickSmokeSymbol(spotRaw)
}

// TradeSymbol resolves the trade symbol from a live client.
func TradeSymbol(t *testing.T, client *polyester.Client, ctx context.Context) string {
	t.Helper()
	return PickTradeSymbol(spotRawFromClient(t, client, ctx))
}

// HydrateSpotRaw fetches spot config and hydrates catalogs.
func HydrateSpotRaw(ctx context.Context, client *polyester.Client) (map[string]any, error) {
	cfg, err := client.MarketData.GetSpotConfig(ctx)
	if err != nil {
		return nil, err
	}
	if client.Catalogs != nil {
		client.Catalogs.HydrateSpotConfig(cfg.Raw)
	}
	return cfg.Raw, nil
}

func spotRawFromClient(t *testing.T, client *polyester.Client, ctx context.Context) map[string]any {
	t.Helper()
	raw, err := HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
