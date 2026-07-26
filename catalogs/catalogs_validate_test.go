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
