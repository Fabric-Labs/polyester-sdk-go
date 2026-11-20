package decode

import (
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// ServedDepthTier checks the tier a reply names as served against the tier
// requested: the contract serves the requested tier and names it in every
// reply, available or not. A reply naming none, an unknown tier or another
// tier is an *errors.ResponseContractError of operation.
func ServedDepthTier(operation string, requested, served typev1.DepthTier) error {
	if served == requested {
		return nil
	}
	if _, known := codecs.LevelsForDepthTier(served); !known {
		return &sdkerrors.ResponseContractError{Operation: operation, Msg: fmt.Sprintf("the reply names no known served depth tier (%d), requested %s", int32(served), requested)}
	}
	return &sdkerrors.ResponseContractError{Operation: operation, Msg: fmt.Sprintf("the reply was served at %s, requested %s", served, requested)}
}

// OrderbookFromProto decodes an Orderbook.Get snapshot requested at depth
// levels per side, a tier (codecs.OrderbookDepthTiers). Its Depth is that
// tier; a reply that names another served tier, or none, is an
// *errors.ResponseContractError, and so is a depth that is no tier. Its Ts is
// the snapshot's timestamp, nil when the reply has none.
func OrderbookFromProto(msg *orderbookv1.GetOrderBookResponse, symbol string, depth, quantityScale int) (models.OrderbookData, error) {
	requested, err := codecs.DepthTierForLevels(depth)
	if err != nil {
		return models.OrderbookData{}, err
	}
	if err := ServedDepthTier("GetOrderBook", requested, msg.GetDepth()); err != nil {
		return models.OrderbookData{}, err
	}
	bids, err := levelsFromProto(msg.GetBids(), symbol, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	asks, err := levelsFromProto(msg.GetAsks(), symbol, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	return models.OrderbookData{
		Symbol:    symbol,
		Depth:     depth,
		BookSeq:   strconv.FormatUint(msg.GetBookSeq(), 10),
		BookEpoch: strconv.FormatUint(msg.GetBookEpoch(), 10),
		Bids:      bids,
		Asks:      asks,
		Available: msg.GetAvailable(),
		Ts:        optionalTime(msg.GetTs()),
	}, nil
}

func levelsFromProto(levels []*orderbookv1.PriceLevel, symbol string, scale int) ([]models.OrderbookLevel, error) {
	out := make([]models.OrderbookLevel, 0, len(levels))
	for _, l := range levels {
		if l == nil {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level is missing"}
		}
		if l.GetPriceTicks() <= 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing price"}
		}
		if l.GetQtyScaled() <= 0 {
			return nil, &sdkerrors.ValidationError{Msg: "orderbook level has invalid or missing quantity"}
		}
		out = append(out, models.OrderbookLevel{
			Price: codecs.DecodePriceTicks(l.GetPriceTicks(), symbol),
			Qty:   codecs.DecodeQtyScaled(l.GetQtyScaled(), scale, symbol, nil),
		})
	}
	return out, nil
}
