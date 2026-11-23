package decode

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

// Each index price column, the source column included, must have one entry
// per row; a misaligned response is refused like a misaligned
// GetCandlesColumns one.
func TestIndexPriceCandlesRefuseMisalignedColumns(t *testing.T) {
	aligned := func() *marketdatav1.GetIndexPriceCandlesResponse {
		return &marketdatav1.GetIndexPriceCandlesResponse{
			TsSec: []uint64{120, 60}, Open: []int64{1, 2}, High: []int64{1, 2}, Low: []int64{1, 2}, Close: []int64{1, 2},
			Source: []marketdatav1.IndexPriceCandleSource{
				marketdatav1.IndexPriceCandleSource_INDEX_PRICE_CANDLE_SOURCE_INDEX,
				marketdatav1.IndexPriceCandleSource_INDEX_PRICE_CANDLE_SOURCE_REFERENCE,
			},
		}
	}
	if _, err := IndexPriceCandlesFromProto(aligned(), 6); err != nil {
		t.Fatalf("aligned response: %v", err)
	}
	for name, cut := range map[string]func(*marketdatav1.GetIndexPriceCandlesResponse){
		"open":   func(m *marketdatav1.GetIndexPriceCandlesResponse) { m.Open = m.Open[:1] },
		"high":   func(m *marketdatav1.GetIndexPriceCandlesResponse) { m.High = nil },
		"low":    func(m *marketdatav1.GetIndexPriceCandlesResponse) { m.Low = append(m.Low, 3) },
		"close":  func(m *marketdatav1.GetIndexPriceCandlesResponse) { m.Close = m.Close[:1] },
		"source": func(m *marketdatav1.GetIndexPriceCandlesResponse) { m.Source = nil },
	} {
		msg := aligned()
		cut(msg)
		var transportErr *sdkerrors.TransportError
		if _, err := IndexPriceCandlesFromProto(msg, 6); !errors.As(err, &transportErr) {
			t.Errorf("%s: err = %T %v, want a TransportError", name, err, err)
		}
	}
}

// An empty page decodes to no rows, keeping the symbol and timeframe the
// response names, with an empty (not nil) candle list.
func TestIndexPriceCandlesEmptyPage(t *testing.T) {
	got, err := IndexPriceCandlesFromProto(&marketdatav1.GetIndexPriceCandlesResponse{SymbolId: 3, Timeframe: marketdatav1.Timeframe_DAY_1}, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got.SymbolID != 3 || got.Timeframe != "1d" || got.Candles == nil || len(got.Candles) != 0 || got.NextPageToken != "" {
		t.Fatalf("result = %+v", got)
	}
}
