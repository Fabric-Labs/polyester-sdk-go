package orderbook

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestApplyDeltaDetectsGap(t *testing.T) {
	bids := BookSide{100: 5}
	asks := BookSide{200: 3}
	delta := models.OrderBookDeltaUpdate{
		BookSeqStart: "5",
		BookSeqEnd:   "6",
		Bids:         []models.PriceQtyPair{{PriceTicks: 100, QtyScaled: 7}},
	}
	_, needsRefresh := ApplyDelta(bids, asks, 3, delta)
	if !needsRefresh {
		t.Fatal("expected gap refresh")
	}
}

func TestApplyDeltaUpdatesBook(t *testing.T) {
	bids := BookSide{100: 5}
	asks := BookSide{200: 3}
	delta := models.OrderBookDeltaUpdate{
		BookSeqStart: "3",
		BookSeqEnd:   "4",
		Bids:         []models.PriceQtyPair{{PriceTicks: 100, QtyScaled: 7}},
	}
	seq, needsRefresh := ApplyDelta(bids, asks, 3, delta)
	if needsRefresh {
		t.Fatal("unexpected refresh")
	}
	if seq != 4 {
		t.Fatalf("seq=%d", seq)
	}
	if bids[100] != 7 {
		t.Fatalf("bids=%v", bids)
	}
}

func TestBuildOrderbookDataBuckets(t *testing.T) {
	bids := BookSide{101: 2, 105: 3}
	asks := BookSide{200: 1}
	data, err := BuildOrderbookData("BTC-USDT", 10, 4, bids, asks, 10, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Bids) == 0 || len(data.Asks) == 0 {
		t.Fatalf("data=%+v", data)
	}
	if data.Bids[0].Price.Ticks != 100 {
		t.Fatalf("bid bucket=%d", data.Bids[0].Price.Ticks)
	}
}

func TestBucketSideRejectsNegativeAndOverflow(t *testing.T) {
	if _, err := BucketSide(BookSide{-1: 1}, 10, false); err == nil {
		t.Fatal("expected negative price rejection")
	}
	if _, err := BucketSide(BookSide{100: int(^uint(0) >> 1), 101: 1}, 10, false); err == nil {
		t.Fatal("expected quantity overflow rejection")
	}
}

func TestBuildOrderbookDataRejectsNegativeLevels(t *testing.T) {
	if _, err := BuildOrderbookData("BTC-USDT", 10, 4, BookSide{-5: 1}, BookSide{200: 1}, 0, 8); err == nil {
		t.Fatal("expected negative level rejection")
	}
}

func TestApplySideDeltaIgnoresNegativeLevels(t *testing.T) {
	book := BookSide{100: 5}
	ApplySideDelta(book, []models.PriceQtyPair{
		{PriceTicks: -1, QtyScaled: 3},
		{PriceTicks: 100, QtyScaled: -2},
		{PriceTicks: 101, QtyScaled: 4},
	})
	if book[100] != 5 || book[101] != 4 {
		t.Fatalf("book=%v", book)
	}
}
