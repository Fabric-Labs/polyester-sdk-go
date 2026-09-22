package catalogs

import "testing"

func TestMarketDataScalesResolveFromSpotConfig(t *testing.T) {
	mgr := NewManager()
	err := mgr.HydrateSpotConfig(map[string]any{
		"assets": []any{map[string]any{
			"asset":                    "BTC",
			"market_data_volume_scale": float64(8),
		}},
		"pairs": []any{map[string]any{
			"symbol":                "BTC-USDT",
			"symbol_id":             float64(1),
			"base_asset":            "BTC",
			"base_quantity_scale":   float64(6),
			"reference_price_scale": float64(8),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	volume, ok := mgr.MarketDataVolumeScaleForSymbolID(1)
	if !ok || volume != 8 {
		t.Fatalf("volume scale=%d ok=%v", volume, ok)
	}
	price, ok := mgr.ReferencePriceScaleForSymbolID(1)
	if !ok || price != 8 {
		t.Fatalf("reference price scale=%d ok=%v", price, ok)
	}
	if _, ok := mgr.MarketDataVolumeScaleForSymbolID(9); ok {
		t.Fatal("unknown symbol should not resolve a volume scale")
	}
}

func TestMarketDataVolumeScaleZeroIsPresent(t *testing.T) {
	mgr := NewManager()
	err := mgr.HydrateSpotConfig(map[string]any{
		"assets": []any{map[string]any{
			"asset":                 "WHOLE",
			"marketDataVolumeScale": float64(0),
		}},
		"pairs": []any{map[string]any{
			"symbol":              "WHOLE-USDT",
			"symbolId":            float64(4),
			"baseAsset":           "WHOLE",
			"base_quantity_scale": float64(0),
			"referencePriceScale": float64(0),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	volume, ok := mgr.MarketDataVolumeScaleForSymbolID(4)
	if !ok || volume != 0 {
		t.Fatalf("volume scale=%d ok=%v", volume, ok)
	}
	price, ok := mgr.ReferencePriceScaleForSymbolID(4)
	if !ok || price != 0 {
		t.Fatalf("reference price scale=%d ok=%v", price, ok)
	}
}
