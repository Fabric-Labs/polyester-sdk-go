package testutil

import (
	"os"
	"strings"
)

var smokeSymbolCandidates = []string{"ETH-USDT", "BTC-USDT", "SOL-USDT", "BNB-USDT"}

const defaultSmokeSymbol = "ETH-USDT"

// EnvSmokeSymbol returns an explicit smoke symbol override from env when set.
func EnvSmokeSymbol() string {
	if v := strings.TrimSpace(os.Getenv("POLYESTER_TEST_SMOKE_SYMBOL")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("POLYESTER_SMOKE_SYMBOL"))
}

// PickSmokeSymbol chooses a liquid pair from spot config, mirroring the Python helper.
func PickSmokeSymbol(spotRaw map[string]any) string {
	if override := EnvSmokeSymbol(); override != "" {
		return override
	}
	symbols := spotSymbols(spotRaw)
	available := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		available[sym] = struct{}{}
	}
	for _, candidate := range smokeSymbolCandidates {
		if _, ok := available[candidate]; ok {
			return candidate
		}
	}
	if len(symbols) > 0 {
		return symbols[0]
	}
	return defaultSmokeSymbol
}

func spotSymbols(spotRaw map[string]any) []string {
	var out []string
	for _, key := range []string{"pairs", "symbols"} {
		raw, _ := spotRaw[key].([]any)
		for _, item := range raw {
			pair, _ := item.(map[string]any)
			if sym, _ := pair["symbol"].(string); sym != "" {
				out = append(out, sym)
			}
		}
	}
	return out
}
