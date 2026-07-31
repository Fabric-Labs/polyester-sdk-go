package codecs

import (
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderIntentSupportsBaseOrQuoteBudgetSizing(t *testing.T) {
	symbol := "BTC-USDT"
	tif := "ioc"
	price := models.PriceFromTicksInt(50_000_000_000)
	budget := models.QtyFromQuoteScaled(500, 6)
	feeAsset := "base"

	intent, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol:              &symbol,
		Side:                "buy",
		OrderType:           "limit",
		TIF:                 &tif,
		MaxQuoteDebitScaled: budget,
		Price:               &price,
		FeeAsset:            &feeAsset,
	}, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if intent.GetMaxQuoteDebitScaled() != 500 || intent.GetBaseQtyScaled() != 0 ||
		intent.GetFeeAsset() != orderv1.FeeAsset_BASE || intent.GetLimitIoc() == nil {
		t.Fatalf("intent=%+v", intent)
	}

	preview, err := PreviewOrderToProto(models.CreateOrderRequest{
		Symbol:              &symbol,
		Side:                "buy",
		OrderType:           "limit",
		TIF:                 &tif,
		MaxQuoteDebitScaled: budget,
		Price:               &price,
		FeeAsset:            &feeAsset,
	}, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	order := preview.GetOrder()
	if order == nil {
		t.Fatal("preview.order missing")
	}
	if order.GetMaxQuoteDebitScaled() != 500 || order.GetFeeAsset() != orderv1.FeeAsset_BASE ||
		order.GetLimitIoc() == nil || order.GetSide() != orderv1.Side_BUY {
		t.Fatalf("preview.order=%+v", order)
	}
}

func TestOrderIntentRejectsAmbiguousOrInvalidQuoteBudgetSizing(t *testing.T) {
	symbol := "BTC-USDT"
	qty := models.QtyFromDecimal("1")
	price := models.PriceFromTicksInt(50_000_000_000)
	budget := models.QtyFromQuoteScaled(500, 6)
	for _, req := range []models.CreateOrderRequest{
		{Symbol: &symbol, Side: "buy", OrderType: "limit", Qty: qty, MaxQuoteDebitScaled: budget, Price: &price},
		{Symbol: &symbol, Side: "sell", OrderType: "market", MaxQuoteDebitScaled: budget},
	} {
		if _, err := OrderIntentToProto(req, 8, 6); err == nil {
			t.Fatalf("expected sizing validation error for %+v", req)
		}
	}
}

func TestOrderIntentRejectsQuoteBudgetScaleMismatch(t *testing.T) {
	symbol := "BTC-USDT"
	tif := "ioc"
	price := models.PriceFromTicksInt(50_000_000_000)
	budget := models.QtyFromQuoteScaled(5_000_000, 8)
	_, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
		MaxQuoteDebitScaled: budget, Price: &price,
	}, 8, 6)
	if err == nil {
		t.Fatal("expected scale mismatch")
	}
}
