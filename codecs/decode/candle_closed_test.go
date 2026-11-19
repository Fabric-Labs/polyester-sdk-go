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
