package orderbook

import (
	"math"
	"strconv"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Book sequences are uint64 on the wire and in the book: a sequence at or
// above 2^63 continues the book; one beyond uint64 is malformed.
func TestApplyDeltaSequenceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		current    uint64
		start, end string
		wantSeq    uint64
		reset      bool
		outOfSync  bool
	}{
		{"2^53 continues", 1 << 53, "9007199254740992", "9007199254740993", 1<<53 + 1, false, false},
		{"2^63-1 reset", 10, "9223372036854775807", "9223372036854775807", math.MaxInt64, true, false},
		{"2^63 reset", 10, "9223372036854775808", "9223372036854775809", 1<<63 + 1, true, false},
		{"2^63 continues", 1 << 63, "9223372036854775808", "9223372036854775809", 1<<63 + 1, false, false},
		{"2^64-1 reset", 10, "18446744073709551614", "18446744073709551615", math.MaxUint64, true, false},
		{"2^64 malformed", 10, "18446744073709551615", "18446744073709551616", 10, true, true},
		{"stale at a book past 2^63", 1<<63 + 5, "9223372036854775808", "9223372036854775809", 1<<63 + 5, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bids := BookSide{}
			seq, outOfSync := ApplyDelta(bids, BookSide{}, tc.current, models.OrderBookDeltaUpdate{
				BookSeqStart: tc.start, BookSeqEnd: tc.end, Reset: tc.reset,
				Bids: []models.PriceQtyPair{{PriceTicks: 100, QtyScaled: 5}},
			})
			if seq != tc.wantSeq || outOfSync != tc.outOfSync {
				t.Fatalf("got (%d, %v), want (%d, %v)", seq, outOfSync, tc.wantSeq, tc.outOfSync)
			}
			data, err := BuildOrderbookData("BTC-USDT", 10, seq, bids, BookSide{}, 0, 8)
			if err != nil {
				t.Fatal(err)
			}
			if want := strconv.FormatUint(seq, 10); data.BookSeq != want {
				t.Fatalf("BookSeq %q, want %q", data.BookSeq, want)
			}
		})
	}
}
