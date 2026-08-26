package catalogs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderbookPriceBucketsForSymbolReadsSpotMarketdata(t *testing.T) {
	mgr := catalogs.NewManager()
	if err := mgr.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "BTC-USDT",
				"symbol_id":           float64(1),
				"base_quantity_scale": float64(8),
				"marketdata": map[string]any{
					"orderbook_price_buckets": []any{0.01, 0.1, 1.0},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got := mgr.OrderbookPriceBucketsForSymbol("BTC-USDT")
	want := []string{"0.01", "0.1", "1"}
	if len(got) != len(want) {
		t.Fatalf("buckets=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buckets=%v want %v", got, want)
		}
	}
}

func TestSymbolForSymbolIDIsReverseOfSymbolIDForSymbol(t *testing.T) {
	mgr := catalogs.NewManager()
	if err := mgr.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "BTC-USDT",
				"symbol_id":           float64(1),
				"base_quantity_scale": float64(8),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	id := mgr.SymbolIDForSymbol("BTC-USDT")
	if id == nil || *id != 1 {
		t.Fatalf("SymbolIDForSymbol=%v", id)
	}
	symbol := mgr.SymbolForSymbolID(*id)
	if symbol == nil || *symbol != "BTC-USDT" {
		t.Fatalf("SymbolForSymbolID=%v", symbol)
	}
	if got := mgr.SymbolForSymbolID(99); got != nil {
		t.Fatalf("unknown id leaked %v", *got)
	}
}

func TestLedgerIDForAssetUsesTypedZipperCatalog(t *testing.T) {
	mgr := catalogs.NewManager()
	if err := mgr.HydrateZipperConfig(models.DepositWithdrawConfig{
		Assets: []models.ZipperAssetConfig{{Asset: "USDT", LedgerID: 99, QuantityScale: 6}},
	}); err != nil {
		t.Fatal(err)
	}
	id := mgr.LedgerIDForAsset("USDT")
	if id == nil || *id != 99 {
		t.Fatalf("ledger_id=%v want 99", id)
	}
}
