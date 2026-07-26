package testutil

// PickSmokeSymbol is a compatibility alias for the canonical live-test selector.
func PickSmokeSymbol(spotRaw map[string]any) string {
	return PickTradeSymbol(spotRaw)
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
