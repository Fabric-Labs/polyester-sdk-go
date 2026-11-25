package decode_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

// An order book snapshot that is valid apart from one level: a negative
// quantity is refused as a contract violation naming the field, not as the
// invalid-or-missing quantity an omitted (0) one is.
func TestOrderbookFromProtoRefusesANegativeLevelQuantity(t *testing.T) {
	book := func(qty int64) *orderbookv1.GetOrderBookResponse {
		return &orderbookv1.GetOrderBookResponse{
			SymbolId: 1,
			BookSeq:  7,
			Depth:    typev1.DepthTier_DEPTH_50,
			Bids:     []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 5}},
			Asks:     []*orderbookv1.PriceLevel{{PriceTicks: 101, QtyScaled: qty}},
		}
	}
	if _, err := decode.OrderbookFromProto(book(3), "BTC-USD", 50, 8); err != nil {
		t.Fatalf("valid book: %v", err)
	}
	_, err := decode.OrderbookFromProto(book(-3), "BTC-USD", 50, 8)
	requireNegativeQty(t, err, "PriceLevel.qty_scaled", "-3")

	// A negative quantity at an omitted price is refused for its quantity.
	negativeAtNoPrice := book(3)
	negativeAtNoPrice.Asks = []*orderbookv1.PriceLevel{{QtyScaled: -4}}
	_, err = decode.OrderbookFromProto(negativeAtNoPrice, "BTC-USD", 50, 8)
	requireNegativeQty(t, err, "PriceLevel.qty_scaled", "-4")
}

// A market overview row that is valid apart from one best size: a negative
// size is refused whether or not that side has a price.
func TestMarketOverviewRefusesANegativeBestSizeWithAndWithoutAPrice(t *testing.T) {
	row := func() *marketoverviewv1.MarketOverview {
		return &marketoverviewv1.MarketOverview{
			SymbolId:         1,
			LastPriceTicks:   74_900,
			BestBidTicks:     74_800,
			BestBidQtyScaled: 12,
			BestAskTicks:     75_000,
			BestAskQtyScaled: 7,
		}
	}
	if _, err := decode.MarketOverviewEntryFromProto(row(), nil); err != nil {
		t.Fatalf("valid row: %v", err)
	}

	priced := row()
	priced.BestBidQtyScaled = -1
	_, err := decode.MarketOverviewEntryFromProto(priced, nil)
	requireNegativeQty(t, err, "MarketOverview.best_bid_qty_scaled", "-1")

	unpriced := row()
	unpriced.BestBidTicks = 0
	unpriced.BestBidQtyScaled = -1
	_, err = decode.MarketOverviewEntryFromProto(unpriced, nil)
	requireNegativeQty(t, err, "MarketOverview.best_bid_qty_scaled", "-1")

	unpricedAsk := row()
	unpricedAsk.BestAskTicks = 0
	unpricedAsk.BestAskQtyScaled = -2
	_, err = decode.MarketOverviewListFromProto(&marketoverviewv1.ListMarketOverviewResponse{Markets: []*marketoverviewv1.MarketOverview{unpricedAsk}}, nil)
	requireNegativeQty(t, err, "MarketOverview.best_ask_qty_scaled", "-2")

	// An empty side with no size still reads as no side.
	empty := row()
	empty.BestAskTicks, empty.BestAskQtyScaled = 0, 0
	got, err := decode.MarketOverviewEntryFromProto(empty, nil)
	if err != nil || got.BestAsk != nil || got.BestAskQty != nil {
		t.Fatalf("empty side = %v, %v, err %v", got.BestAsk, got.BestAskQty, err)
	}
}

func TestPreviewOrderRefusesANegativeResolvedBaseQuantity(t *testing.T) {
	sid := uint32(3)
	preview := func(qty int64) *orderv1.PreviewOrderResponse {
		return &orderv1.PreviewOrderResponse{
			Admissible:            proto.Bool(true),
			ResolvedBaseQtyScaled: proto.Int64(qty),
			EvaluatedAt:           timestamppb.New(timestamppb.Now().AsTime()),
		}
	}
	got, err := decode.PreviewOrderFromProto(preview(0), 8, "BTC-USDT", &sid)
	if err != nil || got.ResolvedBaseQty == nil || got.ResolvedBaseQty.Scaled() != 0 {
		t.Fatalf("zero resolved quantity = %+v, err %v", got.ResolvedBaseQty, err)
	}
	_, err = decode.PreviewOrderFromProto(preview(-8), 8, "BTC-USDT", &sid)
	requireNegativeQty(t, err, "PreviewOrderResponse.resolved_base_qty_scaled", "-8")
}

func TestBatchCreateRefusesANegativeResolvedBaseQuantity(t *testing.T) {
	batch := func(qty int64) *orderv1.BatchCreateOrdersResponse {
		return &orderv1.BatchCreateOrdersResponse{
			Results: []*orderv1.BatchCreateResultItem{{
				ClientOrderId: "a",
				Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{
					OrderId: 9, ResolvedBaseQtyScaled: qty,
				}},
			}},
			AcceptedCount: 1,
		}
	}
	got, err := decode.BatchCreateFromProto(batch(5))
	if err != nil || got.Results[0].ResolvedBaseQty.Scaled() != 5 {
		t.Fatalf("valid batch = %+v, err %v", got, err)
	}
	_, err = decode.BatchCreateFromProto(batch(-9))
	requireNegativeQty(t, err, "BatchCreateResultItem.accepted.resolved_base_qty_scaled", "-9")
}
