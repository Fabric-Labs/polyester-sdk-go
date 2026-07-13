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
	data := BuildOrderbookData("BTC-USDT", 10, 4, bids, asks, 10, 8)
	if len(data.Bids) == 0 || len(data.Asks) == 0 {
		t.Fatalf("data=%+v", data)
	}
}
