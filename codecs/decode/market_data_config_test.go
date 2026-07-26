package decode

import (
	"testing"

	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

func TestSpotConfigFromProtoPreservesValidZeroQuantityScale(t *testing.T) {
	got := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{
			Symbol:            "WHOLE-USDT",
			SymbolId:          9,
			BaseQuantityScale: 0,
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
}
