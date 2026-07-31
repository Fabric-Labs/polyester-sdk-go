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
}

func TestCandlesColumnsRejectsShortParallelArrays(t *testing.T) {
	_, err := CandlesColumnsFromProto(&marketdatav1.GetCandlesColumnsResponse{
		TsSec:  []uint64{1, 2},
		Open:   []int64{1, 2},
		High:   []int64{1},
		Low:    []int64{1, 2},
		Close:  []int64{1, 2},
		Volume: []int64{1, 2},
	}, 8)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
}
