package services

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"google.golang.org/protobuf/proto"
)

func marketDataCatalogs(t *testing.T) *catalogs.Manager {
	t.Helper()
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{
		"assets": []any{
			map[string]any{"asset": "BTC", "quantityScale": float64(8), "marketDataVolumeScale": float64(8)},
			map[string]any{"asset": "BONK", "quantityScale": float64(0), "marketDataVolumeScale": float64(0)},
		},
		"pairs": []any{
			map[string]any{
				"symbol": "BTC-USDT", "symbolId": float64(1), "baseAsset": "BTC", "quoteAsset": "USDT", "baseQuantityScale": float64(8),
				"marketdata": map[string]any{"orderbookPriceBuckets": []any{0.01, 0.1, float64(1), float64(10), float64(100)}},
			},
			map[string]any{
				"symbol": "BONK-USDT", "symbolId": float64(7), "baseAsset": "BONK", "quoteAsset": "USDT", "baseQuantityScale": float64(0),
				"marketdata": map[string]any{"orderbookPriceBuckets": []any{1e-9, 1e-8, 1e-7, 1e-6, 1e-5}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return cats
}

// POLY-8785 parity: Go keeps every digit of sub-micro price buckets.
func TestOrderbookPriceBucketsKeepSubMicroIncrements(t *testing.T) {
	cats := marketDataCatalogs(t)
	for symbol, want := range map[string][]string{
		"BONK-USDT": {"0.000000001", "0.00000001", "0.0000001", "0.000001", "0.00001"},
		"BTC-USDT":  {"0.01", "0.1", "1", "10", "100"},
	} {
		got := cats.OrderbookPriceBucketsForSymbol(symbol)
		if len(got) != len(want) {
			t.Fatalf("%s: %q, want %q", symbol, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: %q, want %q", symbol, got, want)
			}
		}
	}
}

// POLY-8783 parity: Go decodes live overview rows with the catalogs, so they
// keep the symbol and formatted base volume list rows have.
func TestLiveOverviewRowsAreEnrichedFromTheCatalogs(t *testing.T) {
	volume := int64(250)
	payload, err := proto.Marshal(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, Volume_24HBaseScaled: &volume}}})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := decode.MarketOverviewBatchDecoder(marketDataCatalogs(t))(payload)
	if err != nil {
		t.Fatal(err)
	}
	row := batch.Markets[0]
	if row.Symbol != "BTC-USDT" || row.Volume24HBase == nil || *row.Volume24HBase != "0.0000025" {
		t.Fatalf("live row %+v", row)
	}
}
