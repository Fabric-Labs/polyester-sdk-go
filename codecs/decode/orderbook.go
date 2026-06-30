package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func OrderbookFromProto(msg *orderbookv1.GetOrderBookResponse, symbol string, depth, quantityScale int) models.OrderbookData {
	return models.OrderbookData{Symbol: symbol, Depth: depth, BookSeq: strconv.FormatUint(msg.GetBookSeq(), 10), Bids: levelsFromProto(msg.GetBids(), quantityScale), Asks: levelsFromProto(msg.GetAsks(), quantityScale)}
}

func levelsFromProto(levels []*orderbookv1.PriceLevel, scale int) []models.OrderbookLevel {
	out := make([]models.OrderbookLevel, 0, len(levels))
	for _, l := range levels {
		out = append(out, models.OrderbookLevel{
			PriceTicks: strconv.FormatInt(l.GetPriceTicks(), 10), QtyScaled: strconv.FormatInt(l.GetQtyScaled(), 10),
			Price: codecs.FormatPriceTicks(l.GetPriceTicks()), Qty: codecs.FormatQtyScaled(l.GetQtyScaled(), scale),
		})
	}
	return out
}
