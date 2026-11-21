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

// From a server that predates is_closed, the column is empty and only the
// newest primary row (the last, as columns are oldest-first) of a first page
// read with incomplete candles is not known to be closed. Rows of a continuation page, which pages
// only older candles, every reference row, and every row without incomplete
// candles are closed.
func TestCandlesColumnsFromProtoWithoutIsClosedFollowsRequest(t *testing.T) {
	msg := &marketdatav1.GetCandlesColumnsResponse{
		SymbolId: 1, Timeframe: marketdatav1.Timeframe_MIN_1,
		TsSec: []uint64{60, 120, 180}, Open: []int64{1, 1, 1}, High: []int64{1, 1, 1},
		Low: []int64{1, 1, 1}, Close: []int64{1, 1, 1}, Volume: []int64{1, 1, 1},
		ReferenceTsSec: []uint64{60, 120}, ReferenceOpen: []int64{1, 1}, ReferenceHigh: []int64{1, 1},
		ReferenceLow: []int64{1, 1}, ReferenceClose: []int64{1, 1}, ReferenceVolume: []int64{1, 1},
	}
	for _, includeIncomplete := range []bool{false, true} {
		for _, pageToken := range []string{"", "cursor-2"} {
			result, err := CandlesColumnsFromProto(msg, 8, 9, includeIncomplete, pageToken)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Candles) != 3 || len(result.ReferenceCandles) != 2 {
				t.Fatalf("%d rows and %d reference rows, want 3 and 2", len(result.Candles), len(result.ReferenceCandles))
			}
			for i, row := range result.Candles {
				want := !includeIncomplete || pageToken != "" || i != len(result.Candles)-1
				if row.IsClosed != want {
					t.Errorf("include_incomplete=%t page_token=%q candle ts %d: is_closed %t, want %t", includeIncomplete, pageToken, row.TsSec, row.IsClosed, want)
				}
			}
			for _, row := range result.ReferenceCandles {
				if !row.IsClosed {
					t.Errorf("include_incomplete=%t page_token=%q reference ts %d: is_closed false, want true", includeIncomplete, pageToken, row.TsSec)
				}
			}
		}
	}
}

// A server that sends is_closed decides each primary row's flag, whatever the
// request asked; reference rows stay closed, and a column of the wrong length
// is refused.
func TestCandlesColumnsFromProtoReadsServerIsClosed(t *testing.T) {
	msg := &marketdatav1.GetCandlesColumnsResponse{
		SymbolId: 1, Timeframe: marketdatav1.Timeframe_MIN_1,
		TsSec: []uint64{60, 120, 180}, Open: []int64{1, 1, 1}, High: []int64{1, 1, 1},
		Low: []int64{1, 1, 1}, Close: []int64{1, 1, 1}, Volume: []int64{1, 1, 1},
		IsClosed:       []bool{true, false, true},
		ReferenceTsSec: []uint64{60}, ReferenceOpen: []int64{1}, ReferenceHigh: []int64{1},
		ReferenceLow: []int64{1}, ReferenceClose: []int64{1}, ReferenceVolume: []int64{1},
	}
	for _, includeIncomplete := range []bool{false, true} {
		result, err := CandlesColumnsFromProto(msg, 8, 9, includeIncomplete, "")
		if err != nil {
			t.Fatal(err)
		}
		for i, row := range result.Candles {
			if row.IsClosed != msg.IsClosed[i] {
				t.Errorf("include_incomplete=%t candle ts %d: is_closed %t, want %t", includeIncomplete, row.TsSec, row.IsClosed, msg.IsClosed[i])
			}
		}
		if !result.ReferenceCandles[0].IsClosed {
			t.Errorf("include_incomplete=%t: reference row not closed", includeIncomplete)
		}
	}
	short := proto.Clone(msg).(*marketdatav1.GetCandlesColumnsResponse)
	short.IsClosed = []bool{true, true}
	if _, err := CandlesColumnsFromProto(short, 8, 9, true, ""); err == nil {
		t.Fatal("misaligned is_closed column decoded")
	}
}
