package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func OrderbookFromProto(msg *orderbookv1.GetOrderBookResponse, symbol string, depth, quantityScale int) models.OrderbookData {
	return models.OrderbookData{
		Symbol:  symbol,
		Depth:   depth,
		BookSeq: strconv.FormatUint(msg.GetBookSeq(), 10),
		Bids:    levelsFromProto(msg.GetBids(), symbol, quantityScale),
		Asks:    levelsFromProto(msg.GetAsks(), symbol, quantityScale),
	}
}

func levelsFromProto(levels []*orderbookv1.PriceLevel, symbol string, scale int) []models.OrderbookLevel {
	out := make([]models.OrderbookLevel, 0, len(levels))
	for _, l := range levels {
		out = append(out, models.OrderbookLevel{
			Price: codecs.DecodePriceTicks(l.GetPriceTicks(), symbol),
			Qty:   codecs.DecodeQtyScaled(l.GetQtyScaled(), scale, symbol, nil),
		})
	}
	return out
}
