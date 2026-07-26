package testutil

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go"
)

// StrictLiveEnabled reports whether soft-skips should fail closed.
func StrictLiveEnabled() bool {
	return EnvTruthy("POLYESTER_TEST_STRICT_LIVE")
}

// SoftSkip skips unless POLYESTER_TEST_STRICT_LIVE is set, in which case it fails.
func SoftSkip(t *testing.T, args ...any) {
	t.Helper()
	if StrictLiveEnabled() {
		t.Fatal(append([]any{"strict live mode rejected soft skip:"}, args...)...)
	}
	t.Skip(args...)
}

// SoftSkipf skips unless POLYESTER_TEST_STRICT_LIVE is set, in which case it fails.
func SoftSkipf(t *testing.T, format string, args ...any) {
	t.Helper()
	if StrictLiveEnabled() {
		t.Fatalf("strict live mode rejected soft skip: "+format, args...)
	}
	t.Skipf(format, args...)
}

// RequireMutation skips unless POLYESTER_TEST_MUTATION is truthy.
func RequireMutation(t *testing.T) {
	t.Helper()
	if !EnvTruthy("POLYESTER_TEST_MUTATION") {
		SoftSkip(t, "Set POLYESTER_TEST_MUTATION=1 to run mutation tests")
	}
}

// RequireFunded skips unless POLYESTER_TEST_FUNDED is truthy.
func RequireFunded(t *testing.T) {
	t.Helper()
	if !EnvTruthy("POLYESTER_TEST_FUNDED") {
		SoftSkip(t, "Set POLYESTER_TEST_FUNDED=1 to run funded tests")
	}
}

// RequireAccountWideCleanup requires an explicit dedicated-account gate before
// a test may call a non-dry-run account-wide cancellation endpoint.
func RequireAccountWideCleanup(t *testing.T) {
	t.Helper()
	if !EnvTruthy("POLYESTER_TEST_ACCOUNT_WIDE_CLEANUP") {
		SoftSkip(t, "Set POLYESTER_TEST_ACCOUNT_WIDE_CLEANUP=1 only for a dedicated test account")
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

// PickTradeSymbol chooses the trade symbol for mutation/funded/realtime tests.
// Canonical env: POLYESTER_TEST_TRADE_SYMBOL. Smoke-symbol env vars are not consulted.
func PickTradeSymbol(spotRaw map[string]any) string {
	if override := EnvTradeSymbol(); override != "" {
		return override
	}
	// Default candidate list (not smoke-env): prefer liquid majors from spot config.
	symbols := spotSymbols(spotRaw)
	available := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		available[sym] = struct{}{}
	}
	for _, candidate := range []string{"ETH-USDT", "BTC-USDT", "SOL-USDT", "BNB-USDT"} {
		if _, ok := available[candidate]; ok {
			return candidate
		}
	}
	if len(symbols) > 0 {
		return symbols[0]
	}
	return "ETH-USDT"
}

// TradeSymbol resolves the trade symbol from a live client and logs it.
func TradeSymbol(t *testing.T, client *polyester.Client, ctx context.Context) string {
	t.Helper()
	symbol := PickTradeSymbol(spotRawFromClient(t, client, ctx))
	t.Logf("trade_symbol=%s", symbol)
	return symbol
}

// HydrateSpotRaw fetches spot config and hydrates catalogs.
func HydrateSpotRaw(ctx context.Context, client *polyester.Client) (map[string]any, error) {
	cfg, err := client.MarketData.GetSpotConfig(ctx)
	if err != nil {
		return nil, err
	}
	if client.Catalogs != nil {
		if err := client.Catalogs.HydrateSpotConfig(cfg.Raw); err != nil {
			return nil, err
		}
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
