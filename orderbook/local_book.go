package orderbook

import (
	"sort"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// BookSide maps price ticks to scaled quantity.
type BookSide map[int]int

// LevelsFromProtoLevels converts protobuf levels into a side map.
func LevelsFromProtoLevels(levels []*orderbookv1.PriceLevel) BookSide {
	book := BookSide{}
	for _, level := range levels {
		qty := int(level.GetQtyScaled())
		if qty == 0 {
			continue
		}
		book[int(level.GetPriceTicks())] = qty
	}
	return book
}

// ApplySideDelta applies delta levels to one side.
func ApplySideDelta(book BookSide, pairs []models.PriceQtyPair) {
	for _, pair := range pairs {
		price, _ := strconv.Atoi(pair.PriceTicks)
		qty, _ := strconv.Atoi(pair.QtyScaled)
		if qty == 0 {
			delete(book, price)
		} else {
			book[price] = qty
		}
	}
}

// BucketSide aggregates levels into price buckets.
func BucketSide(book BookSide, bucketTicks int) BookSide {
	if bucketTicks <= 0 {
		return book
	}
	aggregated := BookSide{}
	for price, qty := range book {
		if qty <= 0 {
			continue
		}
		bucketPrice := (price / bucketTicks) * bucketTicks
		aggregated[bucketPrice] += qty
	}
	return aggregated
}

// ParseBucketTicks parses a bucket string into tick size.
func ParseBucketTicks(bucket string) int {
	if bucket == "" {
		return 0
	}
	ticks, err := codecs.ParsePriceTicks(bucket, "bucket")
	if err != nil {
		return 0
	}
	return int(ticks)
}

// ApplyDelta applies one delta and returns the new book sequence and whether a snapshot refresh is needed.
func ApplyDelta(bids, asks BookSide, currentBookSeq int, delta models.OrderBookDeltaUpdate) (int, bool) {
	if delta.Reset {
		clear(bids)
		clear(asks)
		currentBookSeq = 0
	}
	seqStart, _ := strconv.Atoi(delta.BookSeqStart)
	seqEnd, _ := strconv.Atoi(delta.BookSeqEnd)
	if currentBookSeq != 0 && seqStart > currentBookSeq+1 {
		return currentBookSeq, true
	}
	if seqEnd <= currentBookSeq {
		return currentBookSeq, false
	}
	ApplySideDelta(bids, delta.Bids)
	ApplySideDelta(asks, delta.Asks)
	if seqEnd > currentBookSeq {
		currentBookSeq = seqEnd
	}
	return currentBookSeq, false
}

// BuildOrderbookData renders the current in-memory book.
func BuildOrderbookData(symbol string, depth, bookSeq int, bids, asks BookSide, bucketTicks, quantityScale int) models.OrderbookData {
	return models.OrderbookData{
		Symbol:  symbol,
		Depth:   depth,
		BookSeq: strconv.Itoa(bookSeq),
		Bids:    sideToLevels(bids, "bids", depth, bucketTicks, quantityScale),
		Asks:    sideToLevels(asks, "asks", depth, bucketTicks, quantityScale),
	}
}

func sideToLevels(book BookSide, side string, limit, bucketTicks, quantityScale int) []models.OrderbookLevel {
	view := BucketSide(book, bucketTicks)
	entries := make([][2]int, 0, len(view))
	for price, qty := range view {
		entries = append(entries, [2]int{price, qty})
	}
	sort.Slice(entries, func(i, j int) bool {
		if side == "bids" {
			return entries[i][0] > entries[j][0]
		}
		return entries[i][0] < entries[j][0]
	})
	if limit > len(entries) {
		limit = len(entries)
	}
	out := make([]models.OrderbookLevel, 0, limit)
	for _, entry := range entries[:limit] {
		price := codecs.FormatPriceTicks(int64(entry[0]))
		qty := codecs.FormatQtyScaled(int64(entry[1]), quantityScale)
		priceDisplay := price
		qtyDisplay := qty
		out = append(out, models.OrderbookLevel{
			Price:        price,
			Qty:          qty,
			PriceTicks:   strconv.Itoa(entry[0]),
			QtyScaled:    strconv.Itoa(entry[1]),
			PriceDisplay: &priceDisplay,
			QtyDisplay:   &qtyDisplay,
		})
	}
	return out
}
