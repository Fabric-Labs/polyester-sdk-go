package catalogs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestHydrateSpotConfigRejectsScaleAboveMax(t *testing.T) {
	m := NewManager()
	err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(65535),
			},
		},
	})
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if scale, ok := m.BaseQuantityScaleForSymbol("ETH-USDT"); ok {
		t.Fatalf("must not store rejected scale, got scale=%d", scale)
	}
}

func TestHydrateSpotConfigRejectsU32OverflowSymbolID(t *testing.T) {
	m := NewManager()
	err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(4294967296), // > uint32 max
				"base_quantity_scale": float64(6),
			},
		},
	})
	if err == nil {
		t.Fatal("expected reject for symbol_id > uint32")
	}
	if id := m.SymbolIDForSymbol("ETH-USDT"); id != nil {
		t.Fatalf("must not store truncated symbol_id, got %d", *id)
	}
}

func TestHydrateSpotConfigAcceptsMaxProtocolScale(t *testing.T) {
	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(maxProtocolScale),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	scale, ok := m.BaseQuantityScaleForSymbol("ETH-USDT")
	if !ok || scale != maxProtocolScale {
		t.Fatalf("got scale=%d ok=%v", scale, ok)
	}
}

func TestRejectedSpotRefreshPreservesPreviousSnapshotAndZeroScale(t *testing.T) {
	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(0),
		}},
	}); err != nil {
		t.Fatalf("hydrate initial spot catalog: %v", err)
	}

	err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "BTC-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(8),
			},
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(6),
			},
		},
	})
	if err == nil {
		t.Fatal("expected contradictory refresh to fail")
	}
	scale, ok := m.BaseQuantityScaleForSymbol("BTC-USDT")
	if !ok || scale != 0 {
		t.Fatalf("previous zero scale was not preserved: scale=%d ok=%v", scale, ok)
	}
	id := m.SymbolIDForSymbol("BTC-USDT")
	if id == nil || *id != 1 {
		t.Fatalf("previous symbol id was not preserved: %v", id)
	}
	if got := m.SymbolIDForSymbol("ETH-USDT"); got != nil {
		t.Fatalf("partial refresh leaked ETH-USDT: %v", *got)
	}
	if got := m.SymbolForSymbolID(1); got == nil || *got != "BTC-USDT" {
		t.Fatalf("reverse lookup for symbol_id=1: %v", got)
	}
	if got := m.SymbolForSymbolID(2); got != nil {
		t.Fatalf("reverse lookup leaked unknown id: %v", *got)
	}
}

func TestRejectedZipperRefreshPreservesPreviousSnapshotAndZeroScale(t *testing.T) {
	m := NewManager()
	if err := m.HydrateZipperConfig(map[string]any{
		"assets": []any{map[string]any{
			"asset":         "USDT",
			"ledgerId":      float64(99),
			"quantityScale": float64(0),
		}},
	}); err != nil {
		t.Fatalf("hydrate initial zipper catalog: %v", err)
	}

	err := m.HydrateZipperConfig(map[string]any{
		"assets": []any{
			map[string]any{"asset": "USDT", "ledgerId": float64(7), "quantityScale": float64(6)},
			map[string]any{"asset": "USDC", "ledgerId": float64(7), "quantityScale": float64(6)},
		},
	})
	if err == nil {
		t.Fatal("expected contradictory refresh to fail")
	}
	scale := m.QuantityScaleForAsset("USDT")
	if scale == nil || *scale != 0 {
		t.Fatalf("previous zero scale was not preserved: %v", scale)
	}
	id := m.LedgerIDForAsset("USDT")
	if id == nil || *id != 99 {
		t.Fatalf("previous ledger id was not preserved: %v", id)
	}
	if got := m.LedgerIDForAsset("USDC"); got != nil {
		t.Fatalf("partial refresh leaked USDC: %v", *got)
	}
}

func TestHydrateZipperConfigRejectsBadScale(t *testing.T) {
	m := NewManager()
	err := m.HydrateZipperConfig(models.DepositWithdrawConfig{
		Assets: []models.ZipperAssetConfig{{Asset: "USDT", LedgerID: 1, QuantityScale: 65535}},
	})
	if err == nil {
		t.Fatal("expected reject")
	}
	if m.QuantityScaleForAsset("USDT") != nil {
		t.Fatal("must not store rejected zipper scale")
	}
}
