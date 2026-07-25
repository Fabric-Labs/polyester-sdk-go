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
	m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(6),
			},
		},
	})
	scale, ok := m.BaseQuantityScaleForSymbol("ETH-USDT")
	if !ok || scale != 6 {
		t.Fatalf("expected scale 6, got scale=%d ok=%v", scale, ok)
	}
	idScale, idOK := m.BaseQuantityScaleForSymbolID(2)
	if !idOK || idScale != 6 {
		t.Fatalf("expected id scale 6, got scale=%d ok=%v", idScale, idOK)
	}
}
