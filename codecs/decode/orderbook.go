package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func OrderbookFromProto(msg *orderbookv1.GetOrderBookResponse, symbol string, depth, quantityScale int) (models.OrderbookData, error) {
	bids, err := levelsFromProto(msg.GetBids(), symbol, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	asks, err := levelsFromProto(msg.GetAsks(), symbol, quantityScale)
	if err != nil {
		return models.OrderbookData{}, err
	}
	return models.OrderbookData{
		Symbol:  symbol,
		Depth:   depth,
		BookSeq: strconv.FormatUint(msg.GetBookSeq(), 10),
		Bids:    bids,
		Asks:    asks,
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
