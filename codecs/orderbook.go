package codecs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

// orderbookStreamDepths are the depths the server publishes on the delta stream.
// It also accepts a subscribe at any other depth, and then never sends anything
// on it.
var orderbookStreamDepths = [...]int{1, 20, 50, 200, 500}

// OrderbookDefaultDepth is the depth per side a snapshot holds when its
// request leaves the depth at 0.
const OrderbookDefaultDepth = 50

// orderbookDepthTiers are the depths per side, in levels, of the contract's
// DepthTier values, ascending, each at its value's position less one.
var orderbookDepthTiers = [...]int{1, 5, 10, 20, 50, 100, 200, 500, 1000, 5000}

// OrderbookDepthTiers returns the depths per side an order book snapshot is
// served at, ascending: the contract's DepthTier values.
func OrderbookDepthTiers() []int {
	return append([]int(nil), orderbookDepthTiers[:]...)
}

// DepthTierForLevels returns the contract's DepthTier for depth levels per
// side. A depth that is no tier is an *errors.ValidationError naming the
// tiers: the contract refuses it.
func DepthTierForLevels(depth int) (typev1.DepthTier, error) {
	names := make([]string, 0, len(orderbookDepthTiers))
	for i, tier := range orderbookDepthTiers {
		if tier == depth {
			return typev1.DepthTier(i + 1), nil
		}
		names = append(names, strconv.Itoa(tier))
	}
	return 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook depth must be one of %s levels per side, got %d", strings.Join(names, ", "), depth)}
}

// LevelsForDepthTier returns the depth per side of tier; false for
// DEPTH_UNSPECIFIED or a tier this SDK does not know.
func LevelsForDepthTier(tier typev1.DepthTier) (int, bool) {
	if tier < 1 || int(tier) > len(orderbookDepthTiers) {
		return 0, false
	}
	return orderbookDepthTiers[tier-1], true
}

// OrderbookSnapshotDepth returns the depth per side Orderbook.Get requests
// for depth: depth itself when it is a tier (OrderbookDepthTiers), or
// OrderbookDefaultDepth for 0. Any other depth is an *errors.ValidationError
// before any request: the contract serves the tiers only.
func OrderbookSnapshotDepth(depth int) (int, error) {
	if depth == 0 {
		return OrderbookDefaultDepth, nil
	}
	if _, err := DepthTierForLevels(depth); err != nil {
		return 0, err
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
		"orderbook subscription depth %d is above the deepest published stream tier (%d); use Orderbook.Get for a deeper snapshot, at a depth of OrderbookDepthTiers",
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
