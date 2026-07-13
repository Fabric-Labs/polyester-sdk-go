package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestStrictDecimalRejectsExcessPrecision(t *testing.T) {
	_, err := codecs.ResolvePriceTicks(models.PriceFromDecimal("50000.1234567"), "price", "")
	if err == nil {
		t.Fatal("expected precision error")
	}
	_, err = codecs.ResolveQtyScaled(models.QtyFromDecimal("0.123456789"), 8, "qty", "", nil)
	if err == nil {
		t.Fatal("expected precision error")
	}
}

func TestScaledPathPassThrough(t *testing.T) {
	qty := models.MustQtyScaled(100_000).WithScale(8).WithSymbol("BTC-USD")
	price := models.MustPriceTicks(50_000_000_000)
	price.Symbol = "BTC-USD"
	gotQty, err := codecs.ResolveQtyScaled(models.QtyFromScaled(qty), 8, "qty", "BTC-USD", nil)
	if err != nil || gotQty != 100_000 {
		t.Fatalf("qty=%d err=%v", gotQty, err)
	}
	gotPrice, err := codecs.ResolvePriceTicks(models.PriceFromTicks(price), "price", "BTC-USD")
	if err != nil || gotPrice != 50_000_000_000 {
		t.Fatalf("price=%d err=%v", gotPrice, err)
	}
}

func TestDomainMismatchRejected(t *testing.T) {
	amount := models.MustAssetAmountScaled(100).WithDomain(models.QuantityDomainAsset)
	_, err := codecs.ResolveAssetAmountScaled(
		models.AssetAmountFromScaled(amount),
		8,
		"amount",
		models.QuantityDomainLedgerE18,
		nil,
	)
	if err == nil {
		t.Fatal("expected domain mismatch")
	}
}

func TestSymbolMismatchOnReuse(t *testing.T) {
	qty := models.MustQtyScaled(100_000).WithScale(8).WithSymbol("BTC-USD")
	_, err := codecs.ResolveQtyScaled(models.QtyFromScaled(qty), 8, "qty", "ETH-USD", nil)
	if err == nil {
		t.Fatal("expected symbol mismatch")
	}
	var ve *errors.ValidationError
	if !asValidation(err, &ve) {
		t.Fatalf("err=%v", err)
	}
}

func asValidation(err error, target **errors.ValidationError) bool {
	if err == nil {
		return false
	}
	if v, ok := err.(*errors.ValidationError); ok {
		*target = v
		return true
	}
	return false
}

func TestCreateOrderAcceptsDecimalAndScaled(t *testing.T) {
	symbol := "BTC-USD"
	tif := "gtc"
	price := models.PriceFromDecimal("50000")
	req := models.CreateOrderRequest{
		Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("0.1"), Price: &price,
	}
	proto, err := codecs.CreateOrderToProto(req, 8)
	if err != nil {
		t.Fatal(err)
	}
	if proto.QtyScaled != 10_000_000 || proto.PriceTicks != 50_000_000_000 {
		t.Fatalf("proto=%+v", proto)
	}

	scaledQty := models.MustQtyScaled(10_000_000).WithScale(8).WithSymbol("BTC-USD")
	scaledPrice := models.PriceFromTicks(models.MustPriceTicks(50_000_000_000))
	req2 := models.CreateOrderRequest{
		Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromScaled(scaledQty), Price: &scaledPrice,
	}
	proto2, err := codecs.CreateOrderToProto(req2, 8)
	if err != nil {
		t.Fatal(err)
	}
	if proto2.QtyScaled != proto.QtyScaled || proto2.PriceTicks != proto.PriceTicks {
		t.Fatalf("proto2=%+v proto=%+v", proto2, proto)
	}
}
