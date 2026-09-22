package decode

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

func TestSpotConfigFromProtoPreservesValidZeroQuantityScale(t *testing.T) {
	got := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{
			Symbol:             "WHOLE-USDT",
			SymbolId:           9,
			BaseQuantityScale:  0,
			QuoteQuantityScale: 0,
		}},
	})

	pairs, ok := got.Raw["pairs"].([]any)
	if !ok || len(pairs) != 1 {
		t.Fatalf("unexpected pairs: %#v", got.Raw["pairs"])
	}
	pair, ok := pairs[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected pair: %#v", pairs[0])
	}
	if scale, exists := pair["base_quantity_scale"]; !exists || scale != float64(0) {
		t.Fatalf("valid zero scale was lost: %#v", pair)
	}
	if scale, exists := pair["quote_quantity_scale"]; !exists || scale != float64(0) {
		t.Fatalf("valid zero quote scale was lost: %#v", pair)
	}
	if scale, exists := pair["reference_price_scale"]; !exists || scale != float64(0) {
		t.Fatalf("valid zero reference price scale was lost: %#v", pair)
	}
}

func TestSpotConfigPreservesCanonicalMarketDataScales(t *testing.T) {
	got := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Assets: []*marketdatav1.AssetConfig{{
			Asset:                 "BTC",
			MarketDataVolumeScale: 0,
		}},
		Pairs: []*marketdatav1.PairConfig{{
			Symbol:              "BTC-USDT",
			SymbolId:            1,
			BaseAsset:           "BTC",
			BaseQuantityScale:   8,
			ReferencePriceScale: 8,
		}},
	})
	assets, _ := got.Raw["assets"].([]any)
	asset, _ := assets[0].(map[string]any)
	if scale, exists := asset["market_data_volume_scale"]; !exists || scale != float64(0) {
		t.Fatalf("market_data_volume_scale=%#v", asset)
	}
	pairs, _ := got.Raw["pairs"].([]any)
	pair, _ := pairs[0].(map[string]any)
	if scale := pair["reference_price_scale"]; scale != float64(8) {
		t.Fatalf("reference_price_scale=%#v", pair)
	}
}

func TestReferenceCandlesUseReferencePriceScale(t *testing.T) {
	got, err := CandlesFromProto(&marketdatav1.GetCandlesResponse{
		Candles: []*marketdatav1.CandlePoint{{
			TsSec: 1, Open: 1_000_000, High: 1_000_000, Low: 1_000_000, Close: 1_000_000, Volume: 100_000_000,
		}},
		ReferenceCandles: []*marketdatav1.CandlePoint{{
			TsSec: 1, Open: 100_000_000, High: 100_000_000, Low: 100_000_000, Close: 100_000_000, Volume: 100_000_000,
		}},
	}, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candles[0].Open != "1" || got.Candles[0].Volume != "1" {
		t.Fatalf("primary candle=%+v", got.Candles[0])
	}
	if got.ReferenceCandles[0].Open != "1" || got.ReferenceCandles[0].Volume != "1" {
		t.Fatalf("reference candle=%+v", got.ReferenceCandles[0])
	}
}

func TestCandlesColumnsRejectsShortParallelArrays(t *testing.T) {
	_, err := CandlesColumnsFromProto(&marketdatav1.GetCandlesColumnsResponse{
		TsSec:  []uint64{1, 2},
		Open:   []int64{1, 2},
		High:   []int64{1},
		Low:    []int64{1, 2},
		Close:  []int64{1, 2},
		Volume: []int64{1, 2},
	}, 8, 6)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
}
