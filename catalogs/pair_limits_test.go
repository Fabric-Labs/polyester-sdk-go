package catalogs

import (
	"math"
	"testing"
)

func TestPairConstraintsExposeOrderLimits(t *testing.T) {
	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{
		"max_open_orders_total": float64(1000),
		"pairs": []any{map[string]any{
			"symbol":                   "BTC-USDT",
			"symbol_id":                float64(1),
			"base_quantity_scale":      float64(8),
			"quote_quantity_scale":     float64(6),
			"tick_size":                "0.01",
			"step_size":                "0.0001",
			"max_limit_qty_base":       "100",
			"max_market_qty_base":      "10.5",
			"max_order_notional_quote": "1000000",
			"max_open_orders_per_pair": float64(200),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	c, ok := m.PairConstraintsForSymbolID(1)
	if !ok {
		t.Fatal("constraints unavailable")
	}
	if c.MaxLimitQtyBase != "100" || c.MaxLimitQtyScaled != 10_000_000_000 ||
		c.MaxMarketQtyBase != "10.5" || c.MaxMarketQtyScaled != 1_050_000_000 ||
		c.MaxOrderNotionalQuote != "1000000" || c.MaxOpenOrdersPerPair != 200 {
		t.Fatalf("limits=%+v", c)
	}
	if c.MaxQuoteSlippage != "9223372036.854775807" || c.MaxQuoteSlippageTicks != math.MaxInt64 {
		t.Fatalf("max quote slippage=%q ticks=%d", c.MaxQuoteSlippage, c.MaxQuoteSlippageTicks)
	}
	if got := m.MaxOpenOrdersTotal(); got != 1000 {
		t.Fatalf("MaxOpenOrdersTotal=%d", got)
	}
}

func TestPairConstraintsLeaveAbsentLimitsUnset(t *testing.T) {
	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	c, ok := m.PairConstraintsForSymbol("BTC-USDT")
	if !ok {
		t.Fatal("constraints unavailable")
	}
	if c.MaxLimitQtyBase != "" || c.MaxLimitQtyScaled != 0 || c.MaxMarketQtyBase != "" || c.MaxMarketQtyScaled != 0 ||
		c.MaxOrderNotionalQuote != "" || c.MaxOpenOrdersPerPair != 0 || m.MaxOpenOrdersTotal() != 0 {
		t.Fatalf("limits=%+v total=%d", c, m.MaxOpenOrdersTotal())
	}
}

func TestPairConstraintsRejectMalformedLimits(t *testing.T) {
	for name, pair := range map[string]map[string]any{
		"off scale":    {"max_limit_qty_base": "0.000000001"},
		"negative":     {"max_market_qty_base": "-1"},
		"not decimal":  {"max_order_notional_quote": "lots"},
		"exponent":     {"max_order_notional_quote": "1e6"},
		"fraction cap": {"max_open_orders_per_pair": 1.5},
	} {
		pair["symbol"], pair["symbol_id"], pair["base_quantity_scale"] = "BTC-USDT", float64(1), float64(8)
		m := NewManager()
		if err := m.HydrateSpotConfig(map[string]any{"pairs": []any{pair}}); err != nil {
			continue
		}
		if c, ok := m.PairConstraintsForSymbol("BTC-USDT"); ok {
			t.Fatalf("%s: malformed limit accepted: %+v", name, c)
		}
	}
}
