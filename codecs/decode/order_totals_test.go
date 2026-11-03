package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// The 0308Z example: a market buy of up to 6000 USDT that the price limit
// stopped early, on a pair with base scale 8 and quote scale 6.
func spentQuoteBuy() *orderv1.Order {
	budget := int64(6_000_000_000)
	return &orderv1.Order{
		OrderId:                      1,
		SymbolId:                     2,
		Side:                         orderv1.Side_BUY,
		Status:                       orderv1.OrderStatus_CANCELED,
		CumQtyScaled:                 4148504,
		FeeAsset:                     orderv1.FeeAsset_QUOTE,
		SubmittedMaxQuoteDebitScaled: &budget,
		AvgPriceTicks:                2908,
		AvgPriceE18:                  &typev1.U128{Lo: 2908714285714},
		CumNotional:                  &typev1.U128{Hi: 17, Lo: 7963446064217622528},
		FeeTotal: &orderv1.OrderFeeTotal{
			AmountE18: &typev1.U128{Lo: 964674285951840000},
			QtyScaled: 4148504,
		},
	}
}

func TestOrderFromProtoMapsExactExecutionTotals(t *testing.T) {
	got := decode.OrderFromProto(spentQuoteBuy())
	if got.AvgPriceE18 != "2908714285714" || got.AvgPrice() != "0.000002908714285714" || got.AvgPx.Ticks() != 2908 {
		t.Fatalf("avg=%q %q %d", got.AvgPriceE18, got.AvgPrice(), got.AvgPx.Ticks())
	}
	if got.CumNotional != "321558095317280000000" {
		t.Fatalf("cum_notional=%q", got.CumNotional)
	}
	if quote, err := got.CumQuote(8); err != nil || quote != "3215.5809531728" {
		t.Fatalf("cum quote=%q err=%v", quote, err)
	}
	if got.FeeTotal == nil || got.FeeTotal.AmountE18 != "964674285951840000" || got.FeeTotal.IsRebate ||
		got.FeeTotal.Qty.Scaled() != 4148504 {
		t.Fatalf("fee_total=%+v", got.FeeTotal)
	}
	if used, ok := got.QuoteUsed(8); !ok || used != "3216.54562745875184" {
		t.Fatalf("used=%q ok=%v", used, ok)
	}
	if unspent, ok := got.QuoteUnspent(8, 6); !ok || unspent != "2783.45437254124816" {
		t.Fatalf("unspent=%q ok=%v", unspent, ok)
	}
}

func TestOrderTotalsUnsetMeanNoFills(t *testing.T) {
	got := decode.OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 2})
	if got.AvgPriceE18 != "0" || got.AvgPrice() != "0" || got.CumNotional != "0" || got.FeeTotal != nil {
		t.Fatalf("order=%+v", got)
	}
	if quote, err := got.CumQuote(8); err != nil || quote != "0" {
		t.Fatalf("cum quote=%q err=%v", quote, err)
	}
}

func TestQuoteUsedWaitsForEveryFillsFee(t *testing.T) {
	msg := spentQuoteBuy()
	msg.FeeTotal.QtyScaled = 4000000 // a fill's fee is still being recorded
	got := decode.OrderFromProto(msg)
	if _, ok := got.QuoteUsed(8); ok {
		t.Fatal("quote used before every fill's fee is recorded")
	}
	if _, ok := got.QuoteUnspent(8, 6); ok {
		t.Fatal("quote unspent before every fill's fee is recorded")
	}
	msg.FeeTotal = nil // open-order reads and stream updates carry no fee total
	if _, ok := decode.OrderFromProto(msg).QuoteUsed(8); ok {
		t.Fatal("quote used without a fee total")
	}
}

func TestQuoteUsedFollowsFeeAssetAndRebate(t *testing.T) {
	order := decode.OrderFromProto(spentQuoteBuy())
	order.FeeTotal.IsRebate = true
	if used, ok := order.QuoteUsed(8); !ok || used != "3214.61627888684816" {
		t.Fatalf("rebate used=%q ok=%v", used, ok)
	}
	order.FeeTotal.IsRebate = false
	order.FeeAsset = "base" // taken from the base received
	if used, ok := order.QuoteUsed(8); !ok || used != "3215.5809531728" {
		t.Fatalf("base fee used=%q ok=%v", used, ok)
	}
	order.SubmittedMaxQuoteDebitScaled = "" // sized by base quantity
	if _, ok := order.QuoteUnspent(8, 6); ok {
		t.Fatal("unspent needs a quote amount")
	}
	if _, ok := (models.Order{}).QuoteUsed(-1); ok {
		t.Fatal("negative scale")
	}
}
