//go:build race

package catalogs

import (
	"strconv"
	"sync"
	"testing"
)

func TestHydrateSpotConfigCallerMutationIsRaceIsolated(t *testing.T) {
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
	if err := m.HydrateSpotConfig(map[string]any{"pairs": []any{pair}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10_000; i++ {
			buckets[0] = strconv.Itoa(i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10_000; i++ {
			got := m.OrderbookPriceBucketsForSymbol("BTC-USDT")
			if len(got) != 2 || got[0] != "0.01" {
				t.Errorf("published catalog changed during caller mutation: %v", got)
				return
			}
		}
	}()
	wg.Wait()
}

func TestHydrateSpotConfigPointerMutationIsRaceIsolated(t *testing.T) {
	m := NewManager()
	label := "original"
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(8),
			"metadata":            map[string]any{"label": &label},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	publishedPair := m.SpotConfig["pairs"].([]any)[0].(map[string]any)
	publishedLabel := publishedPair["metadata"].(map[string]any)["label"].(*string)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10_000; i++ {
			label = strconv.Itoa(i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10_000; i++ {
			if *publishedLabel != "original" {
				t.Errorf("published pointer changed during caller mutation: %q", *publishedLabel)
				return
			}
		}
	}()
	wg.Wait()
}

func TestHydrateSpotConfigRejectsOpaqueMutableStateWithoutReadingIt(t *testing.T) {
	hidden := map[string]string{"value": "initial"}
	config := map[string]any{
		"pairs": []any{map[string]any{
			"symbol":              "BTC-USDT",
			"symbol_id":           float64(1),
			"base_quantity_scale": float64(8),
			"metadata":            catalogOpaqueMutableMetadata{hiddenMap: hidden},
		}},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10_000; i++ {
			hidden["value"] = strconv.Itoa(i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1_000; i++ {
			if err := NewManager().HydrateSpotConfig(config); err == nil {
				t.Error("mutable unexported state was accepted")
				return
			}
		}
	}()
	wg.Wait()
}
