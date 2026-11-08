package codecs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// orderbookStreamDepths are the depths the server publishes on the delta stream,
// fewer than the snapshot enum. It also accepts a subscribe at any other depth,
// and then never sends anything on it.
var orderbookStreamDepths = [...]int{1, 20, 50, 200, 500}

// orderbookSnapshotDepths are the depths Orderbook.Get can request, ascending.
var orderbookSnapshotDepths = [...]int{1, 5, 10, 20, 50, 100, 200, 500, 1000}

const (
	orderbookSnapshotMaxDepth     = 1000
	orderbookSnapshotDefaultDepth = 50
)

// OrderbookSnapshotDepth returns the snapshot depth Orderbook.Get requests for
// depth: depth itself when it is a snapshot depth, else the smallest one
// covering it, so the snapshot never holds fewer levels than asked. 0 is the
// default 50. A depth below 0 or above 1000 is a ValidationError.
func OrderbookSnapshotDepth(depth int) (int, error) {
	return coveringSnapshotDepth(depth, "orderbook snapshot depth")
}

// coveringSnapshotDepth is depth when it is a snapshot depth, else the smallest
// one covering it; 0 is the default 50. what names the depth in refusals.
func coveringSnapshotDepth(depth int, what string) (int, error) {
	if depth == 0 {
		return orderbookSnapshotDefaultDepth, nil
	}
	if depth < 0 {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("%s must not be negative, got %d", what, depth)}
	}
	for _, tier := range orderbookSnapshotDepths {
		if tier >= depth {
			return tier, nil
		}
	}
	return 0, &errors.ValidationError{Msg: fmt.Sprintf(
		"%s %d is above the deepest snapshot depth (%d)", what, depth, orderbookSnapshotMaxDepth,
	)}
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
		"orderbook subscription depth %d is above the deepest published stream tier (%d); use Orderbook.Get for a snapshot of up to %d levels",
		depth, orderbookStreamDepths[len(orderbookStreamDepths)-1], orderbookSnapshotMaxDepth,
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

func DepthToConnectEnum(depth int) string {
	switch {
	case depth <= 0:
		return "DEPTH_5"
	case depth == 1:
		return "DEPTH_1"
	case depth <= 5:
		return "DEPTH_5"
	case depth <= 10:
		return "DEPTH_10"
	case depth <= 20:
		return "DEPTH_20"
	case depth <= 50:
		return "DEPTH_50"
	case depth <= 100:
		return "DEPTH_100"
	case depth <= 200:
		return "DEPTH_200"
	case depth <= 500:
		return "DEPTH_500"
	default:
		return "DEPTH_1000"
	}
}

func DepthProtoValue(depth int, values map[string]int32) (int32, error) {
	name := DepthToConnectEnum(depth)
	if v, ok := values[name]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unsupported depth %d", depth)
}
