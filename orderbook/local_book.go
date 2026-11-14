package orderbook

import (
	"fmt"
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
func LevelsFromProtoLevels(levels []*orderbookv1.PriceLevel) (BookSide, error) {
	book := BookSide{}
	for _, level := range levels {
		if level == nil {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level is missing"}
		}
		price := int(level.GetPriceTicks())
		qty := int(level.GetQtyScaled())
		if price <= 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
		}
		if qty <= 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing quantity"}
		}
		book[price] = qty
	}
	return book, nil
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
			if price == 0 && qty > 0 {
				return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
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
		// Only a positive raw bid may floor into the zero display boundary.
		if price == 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
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

// ApplyDelta applies one delta to a book at currentBookSeq and returns the
// new book sequence and whether the delta is out of sequence.
//
// Each delta is the difference between two published views of the tier's top
// levels: it applies only to a book at exactly its BookSeqStart, the previous
// delta's BookSeqEnd. A delta whose BookSeqEnd is at most currentBookSeq is
// already in the book and is dropped. A reset carries the whole view and
// replaces the book whenever it advances the sequence. Any other delta (one
// that starts before or after currentBookSeq, a malformed sequence or a
// malformed level) is out of sequence: it is not applied, the book and
// sequence are unchanged, and the book no longer follows the stream until the
// next reset.
func ApplyDelta(bids, asks BookSide, currentBookSeq int, delta models.OrderBookDeltaUpdate) (int, bool) {
	// Reject the whole update atomically. Skipping corrupt rows while advancing
	// the sequence permanently leaves stale quantity in the local book. A
	// positive quantity at price zero is a level BucketSide refuses to render,
	// so it is as malformed as a negative row.
	for _, pair := range append(append([]models.PriceQtyPair{}, delta.Bids...), delta.Asks...) {
		if pair.PriceTicks < 0 || pair.QtyScaled < 0 || (pair.PriceTicks == 0 && pair.QtyScaled > 0) {
			return currentBookSeq, true
		}
	}
	seqStart, startErr := strconv.Atoi(delta.BookSeqStart)
	seqEnd, endErr := strconv.Atoi(delta.BookSeqEnd)
	if startErr != nil || endErr != nil || seqStart < 0 || seqEnd < seqStart {
		return currentBookSeq, true
	}
	// A reset may replace the current snapshot only when it advances the
	// sequence. Validate this before clearing either side so buffered stale or
	// equal resets cannot rewind a newer snapshot.
	if seqEnd <= currentBookSeq {
		return currentBookSeq, false
	}
	// A delta diffs the view at seqStart: applied to any other view, a level
	// that changed in between keeps a stale quantity.
	if !delta.Reset && seqStart != currentBookSeq {
		return currentBookSeq, true
	}
	if delta.Reset {
		clear(bids)
		clear(asks)
	}
	ApplySideDelta(bids, delta.Bids)
	ApplySideDelta(asks, delta.Asks)
	return seqEnd, false
}

// BuildOrderbookData renders the current in-memory book with up to depth levels
// per side; depth 0 renders no levels. A negative depth is a ValidationError.
func BuildOrderbookData(symbol string, depth, bookSeq int, bids, asks BookSide, bucketTicks, quantityScale int) (models.OrderbookData, error) {
	if depth < 0 {
		return models.OrderbookData{}, &sdkerrors.ValidationError{Msg: fmt.Sprintf("orderbook depth must not be negative, got %d", depth)}
	}
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
		if price.Ticks() < 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
		}
		qty := codecs.DecodeQtyScaled(int64(entry[1]), quantityScale, symbol, nil)
		if qty.Scaled() < 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing quantity"}
		}
		out = append(out, models.OrderbookLevel{
			Price: price,
			Qty:   qty,
		})
	}
	return out, nil
}
