package catalogs

import "testing"

func TestEmptyCatalogDoesNotDefaultETHUSDTToScale8(t *testing.T) {
	m := NewManager()
	if scale, ok := m.BaseQuantityScaleForSymbol("ETH-USDT"); ok || scale != 0 {
		t.Fatalf("expected missing scale, got scale=%d ok=%v", scale, ok)
	}
}

func TestHydratedETHUSDTUsesScale6(t *testing.T) {
	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":               "ETH-USDT",
				"symbol_id":            float64(2),
				"base_quantity_scale":  float64(6),
				"quote_quantity_scale": float64(6),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	scale, ok := m.BaseQuantityScaleForSymbol("ETH-USDT")
	if !ok || scale != 6 {
		t.Fatalf("expected scale 6, got scale=%d ok=%v", scale, ok)
	}
	idScale, idOK := m.BaseQuantityScaleForSymbolID(2)
	if !idOK || idScale != 6 {
		t.Fatalf("expected id scale 6, got scale=%d ok=%v", idScale, idOK)
	}
	quoteScale, quoteOK := m.QuoteQuantityScaleForSymbol("ETH-USDT")
	if !quoteOK || quoteScale != 6 {
		t.Fatalf("expected quote scale 6, got scale=%d ok=%v", quoteScale, quoteOK)
	}
	quoteIDScale, quoteIDOK := m.QuoteQuantityScaleForSymbolID(2)
	if !quoteIDOK || quoteIDScale != 6 {
		t.Fatalf("expected quote id scale 6, got scale=%d ok=%v", quoteIDScale, quoteIDOK)
	}
}

func TestEmptyCatalogQuoteScaleMissing(t *testing.T) {
	m := NewManager()
	if scale, ok := m.QuoteQuantityScaleForSymbol("NOPE"); ok || scale != 0 {
		t.Fatalf("expected missing quote scale, got scale=%d ok=%v", scale, ok)
	}
	if scale, ok := m.QuoteQuantityScaleForSymbolID(999); ok || scale != 0 {
		t.Fatalf("expected missing quote id scale, got scale=%d ok=%v", scale, ok)
	}
}

func TestMalformedQuoteScaleRejectsCatalog(t *testing.T) {
	m := NewManager()
	err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":               "BTC-USDT",
				"symbol_id":            float64(1),
				"base_quantity_scale":  float64(8),
				"quote_quantity_scale": float64(65535),
			},
		},
	})
	if err == nil {
		t.Fatal("expected malformed quote scale rejection")
	}
	if scale, ok := m.QuoteQuantityScaleForSymbol("BTC-USDT"); ok || scale != 0 {
		t.Fatalf("failed hydrate must not publish quote scale, got scale=%d ok=%v", scale, ok)
	}
}
