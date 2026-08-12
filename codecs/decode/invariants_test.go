package decode

import (
	"strconv"
	"testing"

	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestMillisecondShapedTimestampsDecodeSuccessfully(t *testing.T) {
	ms := uint64(1_700_000_000_000)
	result := MarketTradesFromProto(&marketdatav1.GetTradesResponse{
		Trades: []*marketdatav1.MarketTrade{{TsNs: ms, QtyScaled: 1, PriceTicks: 1}},
	}, 8)
	if len(result.Trades) != 1 {
		t.Fatalf("expected one trade, got %d", len(result.Trades))
	}
	if result.Trades[0].TsNs != strconv.FormatUint(ms, 10) {
		t.Fatalf("ts_ns=%q", result.Trades[0].TsNs)
	}

	order := OrderFromProto(&orderv1.Order{OrderId: 7, CreatedTsNs: ms})
	if order.CreatedTsNs != strconv.FormatUint(ms, 10) {
		t.Fatalf("created_ts_ns=%q", order.CreatedTsNs)
	}
}
