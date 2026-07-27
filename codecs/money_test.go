package codecs_test

import (
	"math/big"
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

func TestAssetAmountDeepCopiesBigIntAtBoundaries(t *testing.T) {
	source := big.NewInt(125)
	amount, err := models.NewAssetAmountFromBig(source)
	if err != nil {
		t.Fatal(err)
	}
	source.SetInt64(999)
	if amount.Scaled().Cmp(big.NewInt(125)) != 0 {
		t.Fatal("source big.Int mutated stored amount")
	}
	returned := amount.Scaled()
	returned.SetInt64(777)
	if amount.Scaled().Cmp(big.NewInt(125)) != 0 {
		t.Fatal("returned big.Int mutated stored amount")
	}
	input := models.AssetAmountFromScaled(amount)
	fromInput, _ := input.ScaledValue()
	fromInput.Scaled().SetInt64(555)
	again, _ := input.ScaledValue()
	if again.Scaled().Cmp(big.NewInt(125)) != 0 {
		t.Fatal("input accessor leaked mutable amount")
	}
}

func TestAssetAmountRescalesExactlyToE18(t *testing.T) {
	amount := models.MustAssetAmountScaled(125).
		WithScale(2).
		WithDomain(models.QuantityDomainLedgerE18).
		WithAssetID(7)
	inputScale := 2
	assetID := uint32(7)
	got, err := codecs.ResolveAssetAmountScaledToScale(
		models.AssetAmountFromScaled(amount), &inputScale, 18, "amount",
		models.QuantityDomainLedgerE18, &assetID,
	)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := new(big.Int).SetString("1250000000000000000", 10)
	if got.Cmp(want) != 0 {
		t.Fatalf("amount_e18=%s want %s", got, want)
	}

	inexact, _ := models.NewAssetAmountFromBig(big.NewInt(1))
	inexact = inexact.WithScale(19).WithDomain(models.QuantityDomainLedgerE18)
	if _, err := codecs.ResolveAssetAmountScaledToScale(
		models.AssetAmountFromScaled(inexact), nil, 18, "amount",
		models.QuantityDomainLedgerE18, nil,
	); err == nil {
		t.Fatal("expected inexact downscale rejection")
	}
}

func TestScaledPathPassThrough(t *testing.T) {
	qty := models.MustQtyScaled(100_000).WithScale(8).WithSymbol("BTC-USD")
	price := models.MustPriceTicks(50_000_000_000).WithSymbol("BTC-USD")
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
	intent := proto.GetOrder()
	if intent.GetQtyScaled() != 10_000_000 || intent.GetLimitGtc().GetPriceTicks() != 50_000_000_000 {
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
	intent2 := proto2.GetOrder()
	if intent2.GetQtyScaled() != intent.GetQtyScaled() || intent2.GetLimitGtc().GetPriceTicks() != intent.GetLimitGtc().GetPriceTicks() {
		t.Fatalf("proto2=%+v proto=%+v", proto2, proto)
	}
}
