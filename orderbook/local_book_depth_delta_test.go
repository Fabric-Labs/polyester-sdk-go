package orderbook

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// POLY-8581: a negative depth is invalid caller input for the exported
// renderer, so it is a ValidationError instead of a slice-bounds panic.
func TestTicket8581BuildOrderbookDataRefusesNegativeDepth(t *testing.T) {
	books := map[string][2]BookSide{
		"empty":     {{}, {}},
		"populated": {{100: 5, 99: 2}, {101: 3, 102: 1}},
	}
	for name, book := range books {
		for _, depth := range []int{-1, -2} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s depth %d panicked: %v", name, depth, r)
					}
				}()
				_, err := BuildOrderbookData("BTC-USDT", depth, 4, book[0], book[1], 0, 8)
				var validation *sdkerrors.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("%s depth %d: err=%v, want ValidationError", name, depth, err)
				}
			}()
		}
		// Controls: 0 renders no levels; 1 and 2 trim to that many.
		for _, depth := range []int{0, 1, 2} {
			data, err := BuildOrderbookData("BTC-USDT", depth, 4, book[0], book[1], 0, 8)
			if err != nil {
				t.Fatalf("%s depth %d: %v", name, depth, err)
			}
			want := min(depth, len(book[0]))
			if len(data.Bids) != want || len(data.Asks) != want || data.Depth != depth {
				t.Fatalf("%s depth %d: bids=%d asks=%d depth=%d", name, depth, len(data.Bids), len(data.Asks), data.Depth)
			}
		}
	}
}

// POLY-8583: a positive quantity at price zero is a level the renderer refuses,
// so the whole delta fails closed: no mutation, no sequence advance, refresh.
func TestTicket8583ApplyDeltaRefusesZeroPriceLevel(t *testing.T) {
	zero := models.PriceQtyPair{PriceTicks: 0, QtyScaled: 4}
	valid := models.PriceQtyPair{PriceTicks: 103, QtyScaled: 7}
	for _, tc := range []struct {
		name  string
		delta models.OrderBookDeltaUpdate
	}{
		{"bid", models.OrderBookDeltaUpdate{BookSeqStart: "11", BookSeqEnd: "11", Bids: []models.PriceQtyPair{zero}}},
		{"ask", models.OrderBookDeltaUpdate{BookSeqStart: "11", BookSeqEnd: "11", Asks: []models.PriceQtyPair{zero}}},
		{"mixed bid", models.OrderBookDeltaUpdate{BookSeqStart: "11", BookSeqEnd: "11", Bids: []models.PriceQtyPair{valid, zero}}},
		{"mixed ask", models.OrderBookDeltaUpdate{BookSeqStart: "11", BookSeqEnd: "11", Bids: []models.PriceQtyPair{valid}, Asks: []models.PriceQtyPair{zero}}},
		{"reset", models.OrderBookDeltaUpdate{BookSeqStart: "11", BookSeqEnd: "12", Reset: true, Bids: []models.PriceQtyPair{valid}, Asks: []models.PriceQtyPair{zero}}},
	} {
		bids := BookSide{100: 5}
		asks := BookSide{200: 3}
		seq, refresh := ApplyDelta(bids, asks, 10, tc.delta)
		if seq != 10 || !refresh {
			t.Fatalf("%s: seq=%d refresh=%v, want 10 true", tc.name, seq, refresh)
		}
		if len(bids) != 1 || bids[100] != 5 || len(asks) != 1 || asks[200] != 3 {
			t.Fatalf("%s: book mutated bids=%v asks=%v", tc.name, bids, asks)
		}
		if _, err := BuildOrderbookData("BTC-USDT", 10, seq, bids, asks, 0, 8); err != nil {
			t.Fatalf("%s: book no longer renders: %v", tc.name, err)
		}
	}
}

// Controls for POLY-8583: deletions and valid inserts still apply and advance.
func TestTicket8583ApplyDeltaKeepsValidDeletionsAndInserts(t *testing.T) {
	bids := BookSide{100: 5, 99: 1}
	asks := BookSide{200: 3}
	delta := models.OrderBookDeltaUpdate{
		BookSeqStart: "11",
		BookSeqEnd:   "11",
		Bids:         []models.PriceQtyPair{{PriceTicks: 99, QtyScaled: 0}, {PriceTicks: 0, QtyScaled: 0}},
		Asks:         []models.PriceQtyPair{{PriceTicks: 201, QtyScaled: 2}},
	}
	seq, refresh := ApplyDelta(bids, asks, 10, delta)
	if seq != 11 || refresh {
		t.Fatalf("seq=%d refresh=%v", seq, refresh)
	}
	if len(bids) != 1 || bids[100] != 5 || asks[201] != 2 {
		t.Fatalf("bids=%v asks=%v", bids, asks)
	}
	if _, err := BuildOrderbookData("BTC-USDT", 10, seq, bids, asks, 10, 8); err != nil {
		t.Fatal(err)
	}
}

// POLY-8588 parity: snapshot and rendered levels keep the market they were read for.
func TestTicket8588LevelsAreBoundToTheirMarket(t *testing.T) {
	check := func(name string, levels []models.OrderbookLevel) {
		t.Helper()
		if len(levels) == 0 {
			t.Fatalf("%s: no levels", name)
		}
		for _, level := range levels {
			if level.Price.Symbol() != "LTC-USDT" || level.Qty.Symbol() != "LTC-USDT" {
				t.Fatalf("%s: price %q qty %q", name, level.Price.Symbol(), level.Qty.Symbol())
			}
			if level.Price.CompatibleWith("BTC-USDT") == nil {
				t.Fatalf("%s: cross-market price accepted", name)
			}
			if level.Qty.CompatibleWith(models.QuantityDomainOrderBase, nil, "BTC-USDT", nil) == nil {
				t.Fatalf("%s: cross-market quantity accepted", name)
			}
		}
	}
	snapshot, err := decode.OrderbookFromProto(&orderbookv1.GetOrderBookResponse{
		Bids: []*orderbookv1.PriceLevel{{PriceTicks: 52_320_000_000, QtyScaled: 5}},
		Asks: []*orderbookv1.PriceLevel{{PriceTicks: 52_330_000_000, QtyScaled: 6}},
	}, "LTC-USDT", 5, 8)
	if err != nil {
		t.Fatal(err)
	}
	check("snapshot", append(snapshot.Bids, snapshot.Asks...))
	for _, bucket := range []int{0, 10} {
		rendered, err := BuildOrderbookData("LTC-USDT", 10, 1, BookSide{52_320_000_000: 5}, BookSide{52_330_000_000: 6}, bucket, 8)
		if err != nil {
			t.Fatal(err)
		}
		check("rendered", append(rendered.Bids, rendered.Asks...))
	}
}
