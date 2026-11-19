package decode

import (
	"testing"

	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// GetCandles rows, primary and reference, keep each point's is_closed: the
// still-open bucket reads false and the closed one true.
func TestCandlesFromProtoKeepsIsClosed(t *testing.T) {
	points := func() []*marketdatav1.CandlePoint {
		return []*marketdatav1.CandlePoint{
			{TsSec: 120, Open: 1_000_000_000, High: 1_000_000_000, Low: 1_000_000_000, Close: 1_000_000_000, IsClosed: false},
			{TsSec: 60, Open: 1_000_000_000, High: 1_000_000_000, Low: 1_000_000_000, Close: 1_000_000_000, IsClosed: true},
		}
	}
	result, err := CandlesFromProto(&marketdatav1.GetCandlesResponse{
		SymbolId: 1, Timeframe: marketdatav1.Timeframe_MIN_1,
		Candles: points(), ReferenceCandles: points(),
	}, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	for name, rows := range map[string][]models.Candle{"candles": result.Candles, "reference_candles": result.ReferenceCandles} {
		if len(rows) != 2 {
			t.Fatalf("%s: %d rows, want 2", name, len(rows))
		}
		if rows[0].TsSec != 120 || rows[0].IsClosed {
			t.Errorf("%s: open bucket ts %d is_closed %t, want ts 120 is_closed false", name, rows[0].TsSec, rows[0].IsClosed)
		}
		if rows[1].TsSec != 60 || !rows[1].IsClosed {
			t.Errorf("%s: closed bucket ts %d is_closed %t, want ts 60 is_closed true", name, rows[1].TsSec, rows[1].IsClosed)
		}
	}
}

// The live candle decoder keeps is_closed: an update of the open bucket reads
// false and its terminal update true.
func TestCandlePointFromBytesKeepsIsClosed(t *testing.T) {
	decodePoint := CandlePointFromBytes(1, "1m", 8)
	for _, want := range []bool{false, true} {
		payload, err := proto.Marshal(&marketdatav1.CandlePoint{TsSec: 60, Close: 1_000_000_000, IsClosed: want})
		if err != nil {
			t.Fatal(err)
		}
		candle, err := decodePoint(payload)
		if err != nil {
			t.Fatal(err)
		}
		if candle.IsClosed != want {
			t.Errorf("is_closed %t decoded as %t", want, candle.IsClosed)
		}
	}
}

// The columnar response carries no closed flag. Without incomplete candles
// every row is closed; with them the newest primary and reference rows (the
// last, as columns are oldest-first) read false and earlier rows true.
func TestCandlesColumnsFromProtoIsClosedByIncomplete(t *testing.T) {
	msg := &marketdatav1.GetCandlesColumnsResponse{
		SymbolId: 1, Timeframe: marketdatav1.Timeframe_MIN_1,
		TsSec: []uint64{60, 120, 180}, Open: []int64{1, 1, 1}, High: []int64{1, 1, 1},
		Low: []int64{1, 1, 1}, Close: []int64{1, 1, 1}, Volume: []int64{1, 1, 1},
		ReferenceTsSec: []uint64{60, 120}, ReferenceOpen: []int64{1, 1}, ReferenceHigh: []int64{1, 1},
		ReferenceLow: []int64{1, 1}, ReferenceClose: []int64{1, 1}, ReferenceVolume: []int64{1, 1},
	}
	for _, includeIncomplete := range []bool{false, true} {
		result, err := CandlesColumnsFromProto(msg, 8, 9, includeIncomplete)
		if err != nil {
			t.Fatal(err)
		}
		for name, rows := range map[string][]models.Candle{"candles": result.Candles, "reference_candles": result.ReferenceCandles} {
			for i, row := range rows {
				want := !includeIncomplete || i != len(rows)-1
				if row.IsClosed != want {
					t.Errorf("include_incomplete=%t %s ts %d: is_closed %t, want %t", includeIncomplete, name, row.TsSec, row.IsClosed, want)
				}
			}
		}
	}
}
