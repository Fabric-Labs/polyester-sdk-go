package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Independent fixtures for the published price contract: one price tick is
// 1e-9 quote units. Expected integers are written out by hand, not derived
// from the SDK's scale constant.
var q9Fixtures = []struct {
	pair    string
	decimal string
	ticks   int64
}{
	{"BTC-USDT", "83589", 83589000000000},
	{"ETH-USDT", "2420", 2420000000000},
	{"SOL-USDT", "185.25", 185250000000},
	{"SOL-USDT", "0.000000001", 1},
	{"ETH-USDT", "2420.123456789", 2420123456789},
}

func TestQ9PriceFixturesEncodeAndDecode(t *testing.T) {
	for _, f := range q9Fixtures {
		ticks, err := codecs.ParsePriceTicks(f.decimal, "price")
		if err != nil {
			t.Fatalf("%s %s: %v", f.pair, f.decimal, err)
		}
		if int64(ticks) != f.ticks {
			t.Fatalf("%s encode %s = %d, want %d", f.pair, f.decimal, ticks, f.ticks)
		}
		if got := codecs.FormatPriceTicks(f.ticks); got != f.decimal {
			t.Fatalf("%s decode %d = %q, want %q", f.pair, f.ticks, got, f.decimal)
		}
	}
}

func TestQ9PriceRejectsExcessPrecisionAndOverflow(t *testing.T) {
	for _, raw := range []string{"0.0000000001", "2420.0000000001", "9223372036.854775808", "100000000000"} {
		if ticks, err := codecs.ParsePriceTicks(raw, "price"); err == nil {
			t.Fatalf("ParsePriceTicks(%q) = %d, want error", raw, ticks)
		}
	}
	if ticks, err := codecs.ParsePriceTicks("9223372036.854775807", "price"); err != nil || ticks != 9223372036854775807 {
		t.Fatalf("max int64 price = %d, %v", ticks, err)
	}
}

func TestQ9OrderAndTriggerEncodePaths(t *testing.T) {
	symbol := "ETH-USDT"
	symbolID := uint32(2)
	tif := "gtc"
	price := models.PriceFromDecimal("2420")
	// 0.002315 ETH at base_quantity_scale 6 stays 2315: quantity is not a price.
	create, err := codecs.CreateOrderToProto(models.CreateOrderRequest{
		Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("0.002315"), Price: &price,
	}, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := create.GetOrder().GetLimitGtc().GetPriceTicks(); got != 2420000000000 {
		t.Fatalf("create price_ticks=%d", got)
	}
	if got := create.GetOrder().GetBaseQtyScaled(); got != 2315 {
		t.Fatalf("create base_qty_scaled=%d", got)
	}
	preview, err := codecs.PreviewOrderToProto(models.CreateOrderRequest{
		Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("0.002315"), Price: &price,
	}, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := preview.GetOrder().GetLimitGtc().GetPriceTicks(); got != 2420000000000 {
		t.Fatalf("preview price_ticks=%d", got)
	}

	newPrice := models.PriceFromDecimal("83589")
	modify, err := codecs.ModifyOrderToProto("BTC-USDT", 1, models.OrderKeyByID("100"), nil, nil, &newPrice, nil, nil, nil, nil, 8)
	if err != nil {
		t.Fatal(err)
	}
	if modify.GetNewPriceTicks() != 83589000000000 {
		t.Fatalf("modify new_price_ticks=%d", modify.GetNewPriceTicks())
	}

	trigger := models.PriceFromDecimal("185.25")
	stop, err := codecs.CreateTriggerToProto(3, "SOL-USDT", "stop_loss", &trigger, "sell",
		models.QtyFromDecimal("1.5"), "market", nil, "", "", nil, nil, false, 6, codecs.CreateTriggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stop.GetTrigger().GetStopLoss().GetTriggerPriceTicks(); got != 185250000000 {
		t.Fatalf("stop trigger_price_ticks=%d", got)
	}
	if got := stop.GetTrigger().GetQtyScaled(); got != 1500000 {
		t.Fatalf("stop qty_scaled=%d", got)
	}
	distance := int32(100)
	activation := models.PriceFromDecimal("2420")
	trailing, err := codecs.CreateTriggerToProto(2, symbol, "trailing_stop", nil, "sell",
		models.QtyFromDecimal("0.002315"), "market", nil, "", "", nil, nil, false, 6,
		codecs.CreateTriggerOptions{TrailingDistanceBps: &distance, ActivationPrice: &activation})
	if err != nil {
		t.Fatal(err)
	}
	if got := trailing.GetTrigger().GetTrailingStop().GetActivationPriceTicks(); got != 2420000000000 {
		t.Fatalf("trailing activation_price_ticks=%d", got)
	}
}

func TestQ9DecodePaths(t *testing.T) {
	trades := decode.MarketTradesFromProto(&marketdatav1.GetTradesResponse{Trades: []*marketdatav1.MarketTrade{
		{SymbolId: 1, PriceTicks: 83589000000000, QtyScaled: 12345},
	}}, 8)
	if got := trades.Trades[0].Price.Format(); got != "83589" {
		t.Fatalf("trade price=%q", got)
	}
	if got := trades.Trades[0].Qty.Scaled(); got != 12345 {
		t.Fatalf("trade qty scaled=%d", got)
	}

	userTrade := decode.UserTradeFromProto(&orderv1.UserTrade{PriceTicks: 2420000000000, QtyScaled: 2315})
	if got := userTrade.Price.Format(); got != "2420" {
		t.Fatalf("user trade price=%q", got)
	}

	order := decode.OrderFromProto(&orderv1.Order{PriceTicks: 185250000000, AvgPriceTicks: 2420000000000})
	if order.Price.Format() != "185.25" || order.AvgPx.Format() != "2420" {
		t.Fatalf("order price=%q avg=%q", order.Price.Format(), order.AvgPx.Format())
	}

	book, err := decode.OrderbookFromProto(&orderbookv1.GetOrderBookResponse{
		Bids: []*orderbookv1.PriceLevel{{PriceTicks: 2420000000000, QtyScaled: 2315}},
		Asks: []*orderbookv1.PriceLevel{{PriceTicks: 2420010000000, QtyScaled: 1}},
	}, "ETH-USDT", 1, 6)
	if err != nil {
		t.Fatal(err)
	}
	if book.Bids[0].Price.Format() != "2420" || book.Asks[0].Price.Format() != "2420.01" {
		t.Fatalf("book bid=%q ask=%q", book.Bids[0].Price.Format(), book.Asks[0].Price.Format())
	}

	// Execution candles use Q9. Reference candles keep the pair's own
	// reference_price_scale (8 here); volume keeps the base volume scale (6).
	candles, err := decode.CandlesFromProto(&marketdatav1.GetCandlesResponse{
		Candles:          []*marketdatav1.CandlePoint{{Open: 83589000000000, High: 83600500000000, Low: 83500000000000, Close: 83589000000000, Volume: 1500000}},
		ReferenceCandles: []*marketdatav1.CandlePoint{{Open: 8358900000000, High: 8358900000000, Low: 8358900000000, Close: 8358900000000}},
	}, 6, 8)
	if err != nil {
		t.Fatal(err)
	}
	c := candles.Candles[0]
	if c.Open != "83589" || c.High != "83600.5" || c.Low != "83500" || c.Close != "83589" || c.Volume != "1.5" {
		t.Fatalf("execution candle=%+v", c)
	}
	if r := candles.ReferenceCandles[0]; r.Open != "83589" || r.Close != "83589" {
		t.Fatalf("reference candle=%+v", r)
	}
}
