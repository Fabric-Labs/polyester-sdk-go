package codecs

import (
	"strings"
	"testing"
	"time"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func contractOrder(side, orderType, tif string) models.CreateOrderRequest {
	symbol := "BTC-USDT"
	sid := uint32(1)
	req := models.CreateOrderRequest{Symbol: &symbol, SymbolID: &sid, Side: side, OrderType: orderType}
	if tif != "" {
		req.TIF = &tif
	}
	if orderType == "limit" {
		price := models.PriceFromDecimal("61000")
		req.Price = &price
	}
	return req
}

func mustRefuse(t *testing.T, req models.CreateOrderRequest, want string) {
	t.Helper()
	_, err := OrderIntentToProto(req, 8, 6)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err=%v, want it to mention %q", err, want)
	}
}

// An iceberg display rides on limit GTC and GTD, at the pair's base scale.
func TestOrderIntentEncodesIcebergDisplay(t *testing.T) {
	display := models.QtyFromDecimal("0.1")
	gtc := contractOrder("buy", "limit", "")
	gtc.Qty = models.QtyFromDecimal("1")
	gtc.DisplayQty = &display
	intent, err := OrderIntentToProto(gtc, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := intent.GetLimitGtc().GetDisplayQtyScaled(); got != 10_000_000 {
		t.Fatalf("gtc display=%d", got)
	}

	gtd := contractOrder("sell", "limit", "gtd")
	gtd.Qty = models.QtyFromDecimal("1")
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	gtd.ExpiresAt = &expires
	scaled := models.QtyFromScaledInt(25_000_000)
	gtd.DisplayQty = &scaled
	intent, err = OrderIntentToProto(gtd, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := intent.GetLimitGtd().GetDisplayQtyScaled(); got != 25_000_000 {
		t.Fatalf("gtd display=%d", got)
	}

	plain := contractOrder("buy", "limit", "gtc")
	plain.Qty = models.QtyFromDecimal("1")
	intent, err = OrderIntentToProto(plain, 8, 6)
	if err != nil || intent.GetLimitGtc().GetDisplayQtyScaled() != 0 {
		t.Fatalf("plain intent=%+v err=%v", intent, err)
	}
}

// Market, IOC and FOK orders have no display field; a non-positive display
// is refused before sending. The API owns the other display rules.
func TestOrderIntentRefusesDisplayWhereTheContractHasNone(t *testing.T) {
	display := models.QtyFromDecimal("0.1")
	for _, base := range []models.CreateOrderRequest{
		contractOrder("buy", "market", ""),
		contractOrder("buy", "limit", "ioc"),
		contractOrder("buy", "limit", "fok"),
	} {
		base.Qty = models.QtyFromDecimal("1")
		base.DisplayQty = &display
		mustRefuse(t, base, "display_qty is only valid for limit GTC or GTD orders")
	}
	zero := models.QtyFromScaledInt(0)
	req := contractOrder("buy", "limit", "gtc")
	req.Qty = models.QtyFromDecimal("1")
	req.DisplayQty = &zero
	mustRefuse(t, req, "display_qty must be positive")

	// Display at or above the order's quantity is the API's to refuse.
	large := models.QtyFromDecimal("2")
	req.DisplayQty = &large
	if _, err := OrderIntentToProto(req, 8, 6); err != nil {
		t.Fatalf("display above qty must reach the API: %v", err)
	}
}

// A static peg prices a limit order once at entry from the best bid or ask
// plus signed tick-size increments; the limit price stays the cap or floor.
func TestOrderIntentEncodesStaticPeg(t *testing.T) {
	for _, tc := range []struct {
		reference string
		offset    int64
		want      orderv1.PegReference
	}{
		{"best_bid", 0, orderv1.PegReference_BEST_BID},
		{"BEST_ASK", -3, orderv1.PegReference_BEST_ASK},
	} {
		req := contractOrder("buy", "limit", "gtc")
		req.Qty = models.QtyFromDecimal("0.001")
		req.PostOnly = true
		req.Peg = &models.OrderPeg{Reference: tc.reference, OffsetTicks: tc.offset}
		intent, err := OrderIntentToProto(req, 8, 6)
		if err != nil {
			t.Fatal(err)
		}
		peg := intent.GetPeg()
		if peg.GetReference() != tc.want || peg.GetOffsetTicks() != tc.offset ||
			intent.GetLimitGtc().GetPriceTicks() != 61_000_000_000_000 || !intent.GetLimitGtc().GetPostOnly() {
			t.Fatalf("intent=%+v", intent)
		}
	}
	for _, tif := range []string{"ioc", "fok"} {
		req := contractOrder("sell", "limit", tif)
		req.Qty = models.QtyFromDecimal("0.001")
		req.Peg = &models.OrderPeg{Reference: "best_ask"}
		if intent, err := OrderIntentToProto(req, 8, 6); err != nil || intent.GetPeg() == nil {
			t.Fatalf("%s: intent=%+v err=%v", tif, intent, err)
		}
	}
}

// The contract never combines a peg with market execution, quote sizing or
// attached risk; an unknown reference is refused too.
func TestOrderIntentRefusesPegCombinationsTheContractForbids(t *testing.T) {
	peg := &models.OrderPeg{Reference: "best_bid"}

	market := contractOrder("buy", "market", "")
	market.Qty = models.QtyFromDecimal("1")
	market.Peg = peg
	mustRefuse(t, market, "peg is only valid for limit orders")

	budget := contractOrder("buy", "limit", "ioc")
	budget.MaxQuoteDebitScaled = models.QtyFromQuoteScaled(500, 6)
	budget.Peg = peg
	mustRefuse(t, budget, "peg is only valid for orders sized by qty")

	proceeds := contractOrder("sell", "limit", "ioc")
	proceeds.QuoteProceedsScaled = models.QtyFromQuoteScaled(500, 6)
	proceeds.Peg = peg
	mustRefuse(t, proceeds, "peg is only valid for orders sized by qty")

	risky := contractOrder("buy", "limit", "gtc")
	risky.Qty = models.QtyFromDecimal("1")
	tp := models.PriceFromDecimal("70000")
	risky.AttachedRisk = &models.AttachedRiskInput{TakeProfit: &models.RiskLegInput{TriggerPrice: tp}}
	risky.Peg = peg
	mustRefuse(t, risky, "peg is not valid with attached_risk")

	unknown := contractOrder("buy", "limit", "gtc")
	unknown.Qty = models.QtyFromDecimal("1")
	unknown.Peg = &models.OrderPeg{Reference: "mid"}
	mustRefuse(t, unknown, "peg.reference must be best_bid or best_ask")
}

// A SELL can be sized by the quote it should receive, on market and limit
// IOC orders, at the pair's quote quantity scale.
func TestOrderIntentEncodesQuoteProceedsSizing(t *testing.T) {
	market := contractOrder("sell", "market", "")
	market.QuoteProceedsScaled = models.QtyFromQuoteDecimal("100")
	bps := int32(50)
	market.MaxSlippageBps = &bps
	intent, err := OrderIntentToProto(market, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if intent.GetQuoteProceedsScaled() != 100_000_000 || intent.GetMarketIoc().GetMaxSlippageBps() != 50 ||
		intent.GetBaseQtyScaled() != 0 || intent.GetSide() != orderv1.Side_SELL {
		t.Fatalf("intent=%+v", intent)
	}

	limit := contractOrder("sell", "limit", "ioc")
	limit.QuoteProceedsScaled = models.QtyFromQuoteDecimal("100")
	intent, err = OrderIntentToProto(limit, 8, 6)
	if err != nil || intent.GetQuoteProceedsScaled() != 100_000_000 || intent.GetLimitIoc() == nil {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
	preview, err := PreviewOrderToProto(limit, 8, 6)
	if err != nil || preview.GetOrder().GetQuoteProceedsScaled() != 100_000_000 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
}

// Quote proceeds follow the contract's eligibility rule: SELL, market IOC or
// limit IOC, exactly one sizing, positive, at the catalog quote scale.
func TestOrderIntentRefusesIneligibleQuoteProceeds(t *testing.T) {
	proceeds := models.QtyFromQuoteDecimal("100")

	buy := contractOrder("buy", "market", "")
	buy.QuoteProceedsScaled = proceeds
	mustRefuse(t, buy, "quote_proceeds_scaled is only valid for sell orders")

	for _, tif := range []string{"gtc", "fok"} {
		resting := contractOrder("sell", "limit", tif)
		resting.QuoteProceedsScaled = proceeds
		mustRefuse(t, resting, "quote_proceeds_scaled is only valid for sell market or limit IOC orders")
	}

	both := contractOrder("sell", "market", "")
	both.Qty = models.QtyFromDecimal("1")
	both.QuoteProceedsScaled = proceeds
	mustRefuse(t, both, "exactly one of qty, max_quote_debit_scaled or quote_proceeds_scaled")

	none := contractOrder("sell", "market", "")
	mustRefuse(t, none, "exactly one of qty, max_quote_debit_scaled or quote_proceeds_scaled")

	zero := contractOrder("sell", "market", "")
	zero.QuoteProceedsScaled = models.QtyFromQuoteScaled(0, 6)
	mustRefuse(t, zero, "quote_proceeds_scaled must be positive")

	wrongScale := contractOrder("sell", "market", "")
	wrongScale.QuoteProceedsScaled = models.QtyFromQuoteScaled(100, 8)
	if _, err := OrderIntentToProto(wrongScale, 8, 6); err == nil {
		t.Fatal("a quote amount at another scale must be refused")
	}
}

// DECREMENT self-trade prevention is accepted on orders, batch items and
// trigger creates, attached legs included.
func TestSelfTradePreventionDecrement(t *testing.T) {
	mode := "DECREMENT"
	req := contractOrder("buy", "limit", "gtc")
	req.Qty = models.QtyFromDecimal("1")
	req.SelfTradePreventionMode = &mode
	tp := models.PriceFromDecimal("70000")
	req.AttachedRisk = &models.AttachedRiskInput{TakeProfit: &models.RiskLegInput{TriggerPrice: tp}}
	intent, err := OrderIntentToProto(req, 8, 6)
	if err != nil || intent.GetSelfTradePreventionMode() != orderv1.SelfTradePreventionMode_DECREMENT {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
	batch, err := BatchCreateOrdersToProto([]models.CreateOrderRequest{req}, nil, nil, false, 8, 6)
	if err != nil || batch.GetItems()[0].GetSelfTradePreventionMode() != orderv1.SelfTradePreventionMode_DECREMENT {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	bad := "decrease"
	req.SelfTradePreventionMode = &bad
	mustRefuse(t, req, "expire_taker, expire_maker, expire_both, or decrement")
}

// Each batch-create item carries its own display, peg and quote sizing.
func TestBatchCreateCarriesNewOrderInputs(t *testing.T) {
	display := models.QtyFromDecimal("0.1")
	iceberg := contractOrder("buy", "limit", "gtc")
	iceberg.Qty = models.QtyFromDecimal("1")
	iceberg.DisplayQty = &display
	pegged := contractOrder("buy", "limit", "gtc")
	pegged.Qty = models.QtyFromDecimal("1")
	pegged.Peg = &models.OrderPeg{Reference: "best_bid", OffsetTicks: 1}
	proceeds := contractOrder("sell", "market", "")
	proceeds.QuoteProceedsScaled = models.QtyFromQuoteDecimal("100")
	batch, err := BatchCreateOrdersToProto([]models.CreateOrderRequest{iceberg, pegged, proceeds}, nil, nil, false, 0, 0,
		OrderScales{Base: 8, Quote: 6}, OrderScales{Base: 8, Quote: 6}, OrderScales{Base: 8, Quote: 6})
	if err != nil {
		t.Fatal(err)
	}
	items := batch.GetItems()
	if items[0].GetLimitGtc().GetDisplayQtyScaled() != 10_000_000 ||
		items[1].GetPeg().GetReference() != orderv1.PegReference_BEST_BID || items[1].GetPeg().GetOffsetTicks() != 1 ||
		items[2].GetQuoteProceedsScaled() != 100_000_000 {
		t.Fatalf("items=%+v", items)
	}
}
