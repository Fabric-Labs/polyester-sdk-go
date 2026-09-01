package catalogs

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

type catalogLabels []string

type catalogString string

type catalogMetadata map[string]any

type catalogOpaqueMutableMetadata struct {
	hiddenMap     map[string]string
	hiddenSlice   []string
	hiddenPointer *string
}

type catalogNestedOpaqueMetadata struct {
	hidden catalogOpaqueMutableMetadata
}

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

func TestHydrateCatalogsDeepCloneCallerMapsAndSlices(t *testing.T) {
	m := NewManager()
	buckets := []any{"0.01", "0.1"}
	pair := map[string]any{
		"symbol":              "BTC-USDT",
		"symbol_id":           float64(1),
		"base_quantity_scale": float64(8),
		"marketdata": map[string]any{
			"orderbook_price_buckets": buckets,
		},
	}
	spot := map[string]any{"pairs": []any{pair}}
	if err := m.HydrateSpotConfig(spot); err != nil {
		t.Fatal(err)
	}

	pair["symbol"] = "MUTATED"
	pair["symbol_id"] = float64(99)
	buckets[0] = "999"
	spot["pairs"].([]any)[0] = map[string]any{
		"symbol":              "REPLACED",
		"symbol_id":           float64(100),
		"base_quantity_scale": float64(1),
	}

	id := m.SymbolIDForSymbol("BTC-USDT")
	if id == nil || *id != 1 {
		t.Fatalf("published spot snapshot changed after caller mutation: id=%v", id)
	}
	gotBuckets := m.OrderbookPriceBucketsForSymbol("BTC-USDT")
	if len(gotBuckets) != 2 || gotBuckets[0] != "0.01" || gotBuckets[1] != "0.1" {
		t.Fatalf("nested spot slice was aliased: %v", gotBuckets)
	}

	variant := map[string]any{"zippedAssetId": float64(7)}
	asset := map[string]any{
		"asset": "USDT", "ledgerId": float64(9), "quantityScale": float64(6),
		"variants": []any{variant},
	}
	zipper := map[string]any{"assets": []any{asset}}
	if err := m.HydrateZipperConfig(zipper); err != nil {
		t.Fatal(err)
	}
	asset["ledgerId"] = float64(999)
	variant["zippedAssetId"] = float64(700)
	if id := m.LedgerIDForAsset("USDT"); id == nil || *id != 9 {
		t.Fatalf("published zipper snapshot changed after caller mutation: id=%v", id)
	}

	typed := &models.DepositWithdrawConfig{
		Chains: []models.ZipperChainConfig{{ChainID: 12, Code: "test"}},
		Assets: []models.ZipperAssetConfig{{
			Asset: "USDC", LedgerID: 10, QuantityScale: 6,
			Variants: []models.ZipperAssetChainVariant{{ZippedAssetID: 11, ChainID: 12}},
		}},
	}
	if err := m.HydrateDepositWithdrawConfig(typed); err != nil {
		t.Fatal(err)
	}
	typed.Assets[0].LedgerID = 1000
	typed.Assets[0].Variants[0].ZippedAssetID = 1100
	if id := m.LedgerIDForAsset("USDC"); id == nil || *id != 10 {
		t.Fatalf("published typed zipper snapshot changed after caller mutation: id=%v", id)
	}
	if _, ok := m.QuantityScaleForZippedAssetID(11); !ok {
		t.Fatal("published typed zipper nested slice changed after caller mutation")
	}
}

func TestHydrateSpotConfigDeepClonesNestedPointers(t *testing.T) {
	m := NewManager()
	label := "original"
	labels := catalogLabels{"first", "second"}
	metadata := map[string]any{
		"label":  &label,
		"labels": &labels,
	}
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(8),
			"metadata":            metadata,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	label = "mutated"
	labels[0] = "mutated"
	metadata["extra"] = &label

	publishedPair := m.SpotConfig["pairs"].([]any)[0].(map[string]any)
	publishedMetadata := publishedPair["metadata"].(map[string]any)
	publishedLabel, ok := publishedMetadata["label"].(*string)
	if !ok || publishedLabel == &label || *publishedLabel != "original" {
		t.Fatalf("nested scalar pointer was aliased: type=%T value=%v", publishedMetadata["label"], publishedMetadata["label"])
	}
	publishedLabels, ok := publishedMetadata["labels"].(*catalogLabels)
	if !ok || publishedLabels == &labels || len(*publishedLabels) != 2 || (*publishedLabels)[0] != "first" {
		t.Fatalf("nested slice pointer was aliased or changed type: type=%T value=%v", publishedMetadata["labels"], publishedMetadata["labels"])
	}
	if _, exists := publishedMetadata["extra"]; exists {
		t.Fatal("published metadata map retained caller mutation")
	}
}

func TestHydrateSpotConfigCloneIsCycleSafe(t *testing.T) {
	selfMap := map[string]any{"value": "map"}
	selfMap["self"] = selfMap

	selfSlice := make([]any, 2)
	selfSlice[0] = selfSlice
	selfSlice[1] = "slice"

	var selfPointer any
	selfPointer = &selfPointer

	m := NewManager()
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(8),
			"metadata": map[string]any{
				"map":     selfMap,
				"slice":   selfSlice,
				"pointer": selfPointer,
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	publishedPair := m.SpotConfig["pairs"].([]any)[0].(map[string]any)
	publishedMetadata := publishedPair["metadata"].(map[string]any)

	publishedMap := publishedMetadata["map"].(map[string]any)
	publishedMap["clone-only"] = true
	if _, exists := selfMap["clone-only"]; exists {
		t.Fatal("cycle-safe map clone still aliases its source")
	}
	if self := publishedMap["self"].(map[string]any); self["clone-only"] != true {
		t.Fatal("self-referential map topology was not preserved")
	}

	publishedSlice := publishedMetadata["slice"].([]any)
	publishedSlice[1] = "clone-only"
	if selfSlice[1] != "slice" {
		t.Fatal("cycle-safe slice clone still aliases its source")
	}
	if self := publishedSlice[0].([]any); self[1] != "clone-only" {
		t.Fatal("self-referential slice topology was not preserved")
	}

	publishedPointer := publishedMetadata["pointer"].(*any)
	if publishedPointer == selfPointer.(*any) {
		t.Fatal("cycle-safe pointer clone still aliases its source")
	}
	if nested := (*publishedPointer).(*any); nested != publishedPointer {
		t.Fatal("self-referential pointer topology was not preserved")
	}
}

func TestHydrateSpotConfigRejectsUncloneableMutableUnexportedState(t *testing.T) {
	hidden := "original"
	for _, test := range []struct {
		name     string
		metadata any
	}{
		{
			name:     "map",
			metadata: catalogOpaqueMutableMetadata{hiddenMap: map[string]string{"credential": "must-not-leak"}},
		},
		{
			name:     "slice",
			metadata: catalogOpaqueMutableMetadata{hiddenSlice: []string{"must-not-leak"}},
		},
		{
			name:     "pointer",
			metadata: catalogOpaqueMutableMetadata{hiddenPointer: &hidden},
		},
		{
			name: "nested",
			metadata: catalogNestedOpaqueMetadata{hidden: catalogOpaqueMutableMetadata{
				hiddenMap: map[string]string{"credential": "must-not-leak"},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := NewManager()
			err := m.HydrateSpotConfig(map[string]any{
				"pairs": []any{map[string]any{
					"symbol":              "BTC-USDT",
					"symbol_id":           float64(1),
					"base_quantity_scale": float64(8),
					"metadata":            test.metadata,
				}},
			})
			var validationErr *sdkerrors.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("mutable unexported metadata must be rejected, got %T: %v", err, err)
			}
			if !strings.Contains(validationErr.Error(), "$.pairs[0].metadata") ||
				!strings.Contains(validationErr.Error(), "unexported fields") ||
				!strings.Contains(validationErr.Error(), "JSON-shaped") {
				t.Fatalf("validation error lacks the raw-catalog boundary: %v", validationErr)
			}
			if strings.Contains(validationErr.Error(), "must-not-leak") {
				t.Fatalf("validation error exposed field contents: %v", validationErr)
			}
			if id := m.SymbolIDForSymbol("BTC-USDT"); id != nil {
				t.Fatalf("rejected catalog was published: %v", id)
			}
		})
	}
}

func TestHydrateSpotConfigRejectsLockLikeStructs(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "sync_mutex", value: &sync.Mutex{}},
		{name: "sync_once", value: &sync.Once{}},
		{name: "atomic_int64", value: &atomic.Int64{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := NewManager().HydrateSpotConfig(map[string]any{
				"pairs": []any{map[string]any{
					"symbol":              "BTC-USDT",
					"symbol_id":           float64(1),
					"base_quantity_scale": float64(8),
					"metadata":            map[string]any{"lock_like": test.value},
				}},
			})
			var validationErr *sdkerrors.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("lock/no-copy state must be rejected, got %T: %v", err, err)
			}
			if !strings.Contains(validationErr.Error(), "unexported fields") {
				t.Fatalf("validation error lacks the raw-catalog boundary: %v", validationErr)
			}
		})
	}
}

func TestHydrateSpotConfigAcceptsTimeAndConcreteJSONAliases(t *testing.T) {
	m := NewManager()
	timestamp := time.Date(2026, time.August, 31, 12, 34, 56, 789, time.FixedZone("test", -4*60*60))
	label := catalogString("stable")
	labels := catalogLabels{"first", "second"}
	metadata := catalogMetadata{"label": &label, "labels": labels}
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(8),
			"timestamp":           timestamp,
			"metadata":            metadata,
		}},
	}); err != nil {
		t.Fatalf("time and JSON-compatible aliases should remain accepted: %v", err)
	}
	label = "mutated"
	labels[0] = "mutated"

	pair := m.SpotConfig["pairs"].([]any)[0].(map[string]any)
	if got, ok := pair["timestamp"].(time.Time); !ok || !got.Equal(timestamp) || got.Location() != timestamp.Location() {
		t.Fatalf("time.Time type/value changed: %T %v", pair["timestamp"], pair["timestamp"])
	}
	publishedMetadata, ok := pair["metadata"].(catalogMetadata)
	if !ok {
		t.Fatalf("map alias type changed: %T", pair["metadata"])
	}
	if got, ok := publishedMetadata["label"].(*catalogString); !ok || got == &label || *got != "stable" {
		t.Fatalf("pointer/scalar alias was not cloned with its type: %T %v", publishedMetadata["label"], publishedMetadata["label"])
	}
	if got, ok := publishedMetadata["labels"].(catalogLabels); !ok || len(got) != 2 || got[0] != "first" {
		t.Fatalf("slice alias was not cloned with its type: %T %v", publishedMetadata["labels"], publishedMetadata["labels"])
	}
}
