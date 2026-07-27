package orderbook

import (
	"sort"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// BookSide maps price ticks to scaled quantity.
type BookSide map[int]int

// LevelsFromProtoLevels converts protobuf levels into a side map.
func LevelsFromProtoLevels(levels []*orderbookv1.PriceLevel) BookSide {
	book := BookSide{}
	for _, level := range levels {
		price := int(level.GetPriceTicks())
		qty := int(level.GetQtyScaled())
		if price < 0 || qty <= 0 {
			continue
		}
		book[price] = qty
	}
	return book
}

// ApplySideDelta applies delta levels to one side.
func ApplySideDelta(book BookSide, pairs []models.PriceQtyPair) {
	for _, pair := range pairs {
		price := int(pair.PriceTicks)
		qty := int(pair.QtyScaled)
		// Negative price/qty is wire corruption; never materialize it into the book.
		if price < 0 || qty < 0 {
			continue
		}
		if qty == 0 {
			delete(book, price)
		} else {
			book[price] = qty
		}
	}
}

// BucketSide aggregates levels into executable-side-safe price buckets.
//
// Bids round down; asks round up so displayed asks never appear below their
// executable price.
func BucketSide(book BookSide, bucketTicks int, asks bool) (BookSide, error) {
	if bucketTicks <= 0 {
		for price, qty := range book {
			if price < 0 {
				return nil, &sdkerrors.ValidationError{Msg: "orderbook price ticks must be non-negative"}
			}
			if qty < 0 {
				return nil, &sdkerrors.ValidationError{Msg: "orderbook quantity must be non-negative"}
			}
		}
		return book, nil
	}
	aggregated := BookSide{}
	for price, qty := range book {
		if price < 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook price ticks must be non-negative"}
		}
		if qty <= 0 {
			continue
		}
		floor, err := checkedMul(price/bucketTicks, bucketTicks)
		if err != nil {
			return nil, err
		}
		bucket := floor
		if asks && price%bucketTicks != 0 {
			bucket, err = checkedAdd(floor, bucketTicks)
			if err != nil {
				return nil, &sdkerrors.ValidationError{Msg: "ask bucket price overflow"}
			}
		}
		sum, err := checkedAdd(aggregated[bucket], qty)
		if err != nil {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook bucket quantity overflow"}
		}
		aggregated[bucket] = sum
	}
	return aggregated, nil
}

func checkedMul(a, b int) (int, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	result := a * b
	if result/a != b {
		return 0, &sdkerrors.ValidationError{Msg: "orderbook bucket price overflow"}
	}
	return result, nil
}

func checkedAdd(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, &sdkerrors.ValidationError{Msg: "integer overflow"}
	}
	return result, nil
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
func BuildOrderbookData(symbol string, depth, bookSeq int, bids, asks BookSide, bucketTicks, quantityScale int) (models.OrderbookData, error) {
	bidLevels, err := sideToLevels(bids, "bids", symbol, depth, bucketTicks, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	askLevels, err := sideToLevels(asks, "asks", symbol, depth, bucketTicks, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	return models.OrderbookData{
		Symbol:  symbol,
		Depth:   depth,
		BookSeq: strconv.Itoa(bookSeq),
		Bids:    bidLevels,
		Asks:    askLevels,
	}, nil
}

func sideToLevels(book BookSide, side, symbol string, limit, bucketTicks, quantityScale int) ([]models.OrderbookLevel, error) {
	view, err := BucketSide(book, bucketTicks, side == "asks")
	if err != nil {
		return nil, err
	}
	entries := make([][2]int, 0, len(view))
	for price, qty := range view {
		if price < 0 || qty <= 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid price or quantity"}
		}
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
		price := codecs.DecodePriceTicks(int64(entry[0]), symbol)
		if price.Ticks < 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
		}
		qty := codecs.DecodeQtyScaled(int64(entry[1]), quantityScale, symbol, nil)
		if qty.Scaled < 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing quantity"}
		}
		out = append(out, models.OrderbookLevel{
			Price: price,
			Qty:   qty,
		})
	}
	return out, nil
}
