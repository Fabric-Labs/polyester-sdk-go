package codecs

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// orderbookStreamDepths are the depths the server publishes on the delta stream.
// It also accepts a subscribe at any other depth, and then never sends anything
// on it.
var orderbookStreamDepths = [...]int{1, 20, 50, 200, 500}

// OrderbookDefaultDepth is the depth per side a snapshot holds when its
// request leaves the depth at 0.
const OrderbookDefaultDepth = 50

// OrderbookSnapshotDepth returns the depth per side Orderbook.Get reports for
// depth: depth itself, or OrderbookDefaultDepth for 0. Any level count the
// uint32 request field holds is sent as given: the largest depth the service
// accepts is the MaxDepth every snapshot reports, and the service refuses a
// depth above it with invalid_argument, never truncating it. A negative depth,
// or one above the uint32 range, is a ValidationError.
func OrderbookSnapshotDepth(depth int) (int, error) {
	if depth < 0 {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook snapshot depth must not be negative, got %d", depth)}
	}
	if uint64(depth) > math.MaxUint32 {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook snapshot depth %d is above the uint32 range", depth)}
	}
	if depth == 0 {
		return OrderbookDefaultDepth, nil
	}
	return depth, nil
}

// OrderbookStreamDepths returns the depths published on the delta stream, ascending.
func OrderbookStreamDepths() []int {
	return append([]int(nil), orderbookStreamDepths[:]...)
}

// OrderbookStreamTier returns the smallest published stream depth covering depth,
// for a view trimmed to depth. A depth no tier covers is a ValidationError.
func OrderbookStreamTier(depth int) (int, error) {
	if depth < 1 {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook subscription depth must be positive, got %d", depth)}
	}
	for _, tier := range orderbookStreamDepths {
		if tier >= depth {
			return tier, nil
		}
	}
	return 0, &errors.ValidationError{Msg: fmt.Sprintf(
		"orderbook subscription depth %d is above the deepest published stream tier (%d); use Orderbook.Get for a deeper snapshot, up to the MaxDepth it reports",
		depth, orderbookStreamDepths[len(orderbookStreamDepths)-1],
	)}
}

// RequirePublishedStreamDepth returns depth when the server publishes it. Raw
// deltas of a tier can't be trimmed, so any other depth is a ValidationError.
func RequirePublishedStreamDepth(depth int) (int, error) {
	names := make([]string, 0, len(orderbookStreamDepths))
	for _, tier := range orderbookStreamDepths {
		if tier == depth {
			return depth, nil
		}
		names = append(names, strconv.Itoa(tier))
	}
	return 0, &errors.ValidationError{Msg: fmt.Sprintf(
		"orderbook delta streams are published only at depths %s, got %d; use Orderbook.CreateSubscription for a book trimmed to any depth up to %d",
		strings.Join(names, ", "), depth, orderbookStreamDepths[len(orderbookStreamDepths)-1],
	)}
}
