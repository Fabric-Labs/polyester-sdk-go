package codecs

import (
	stderrors "errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// The order's self-trade prevention mode also governs its attached
// take-profit, stop-loss and trailing stop orders.
func TestOrderIntentEncodesSelfTradePreventionMode(t *testing.T) {
	symbol := "BTC-USDT"
	symbolID := uint32(1)
	price := models.PriceFromDecimal("60000")
	base := models.CreateOrderRequest{
		Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "limit",
		Qty: models.QtyFromScaledInt(100000), Price: &price, PostOnly: true,
		AttachedRisk: attachedOCO("72000", "58000"),
	}
	cases := []struct {
		mode *string
		want orderv1.SelfTradePreventionMode
	}{
		{nil, orderv1.SelfTradePreventionMode_SELF_TRADE_PREVENTION_MODE_UNSPECIFIED},
		{ptr("expire_maker"), orderv1.SelfTradePreventionMode_EXPIRE_MAKER},
		{ptr("expire_taker"), orderv1.SelfTradePreventionMode_EXPIRE_TAKER},
		{ptr("EXPIRE_BOTH"), orderv1.SelfTradePreventionMode_EXPIRE_BOTH},
	}
	for _, tc := range cases {
		req := base
		req.SelfTradePreventionMode = tc.mode
		created, err := CreateOrderToProto(req, 8, 6)
		if err != nil {
			t.Fatal(err)
		}
		preview, err := PreviewOrderToProto(req, 8, 6)
		if err != nil {
			t.Fatal(err)
		}
		batch, err := BatchCreateOrdersToProto([]models.CreateOrderRequest{req}, nil, ptr("b-1"), false, 8, 6)
		if err != nil {
			t.Fatal(err)
		}
		for name, got := range map[string]*orderv1.OrderIntent{
			"create": created.GetOrder(), "preview": preview.GetOrder(), "batch": batch.GetItems()[0],
		} {
			if got.GetSelfTradePreventionMode() != tc.want || got.GetAttachedRisk().GetTakeProfit() == nil {
				t.Fatalf("%s mode=%v want %v (risk=%v)", name, got.GetSelfTradePreventionMode(), tc.want, got.GetAttachedRisk())
			}
		}
	}

	req := base
	req.SelfTradePreventionMode = ptr("expire_none")
	_, err := CreateOrderToProto(req, 8, 6)
	var validation *errors.ValidationError
	if !stderrors.As(err, &validation) {
		t.Fatalf("err=%T %v, want ValidationError", err, err)
	}
}

// A buy sized by a quote amount may carry attached risk: the server checks
// the legs at the quantity the amount buys at the order's price limit.
func TestQuoteAmountBuyCarriesAttachedRisk(t *testing.T) {
	symbol := "BTC-USDT"
	symbolID := uint32(1)
	slippage := int32(200)
	refPrice := models.PriceFromDecimal("77000")
	req, err := CreateOrderToProto(models.CreateOrderRequest{
		Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "market",
		MaxQuoteDebitScaled:  models.QtyFromQuoteScaled(50_000_000, 6),
		MaxSlippageBps:       &slippage,
		MarketClientRefPrice: &refPrice,
		AttachedRisk:         attachedOCO("80000", "74000"),
	}, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	order := req.GetOrder()
	risk := order.GetAttachedRisk()
	if order.GetMaxQuoteDebitScaled() != 50_000_000 || order.GetMarketIoc() == nil || !risk.GetOco() ||
		risk.GetTakeProfit().GetTriggerPriceTicks() != 80_000_000_000_000 ||
		risk.GetStopLoss().GetTriggerPriceTicks() != 74_000_000_000_000 {
		t.Fatalf("order=%v", order)
	}
}
