package catalogs

import (
	"sync"
	"testing"
)

func TestManagerConcurrentHydrateAndRead(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = m.HydrateSpotConfig(map[string]any{
					"pairs": []any{
						map[string]any{
							"symbol":              "BTCUSDT",
							"symbol_id":           float64(1),
							"base_quantity_scale": float64(8 + (n % 3)),
						},
					},
				})
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = m.SymbolIDForSymbol("BTCUSDT")
				_, _ = m.BaseQuantityScaleForSymbol("BTCUSDT")
				_, _ = m.BaseQuantityScaleForSymbolID(1)
			}
		}()
	}
	wg.Wait()
}
