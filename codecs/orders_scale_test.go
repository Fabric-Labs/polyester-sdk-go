package codecs

import (
	"errors"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestQuantityScaleForSymbolRequiresCatalogAndSymbol(t *testing.T) {
	if _, err := QuantityScaleForSymbol(nil, nil); err == nil {
		t.Fatal("expected error for nil catalog and symbol")
	}
	empty := ""
	if _, err := QuantityScaleForSymbol(nil, &empty); err == nil {
		t.Fatal("expected error for empty symbol")
	}
	var ve *sdkerrors.ValidationError
	_, err := QuantityScaleForSymbol(nil, nil)
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}

func TestQuantityScaleForSymbolErrorsWhenCatalogUnhydrated(t *testing.T) {
	m := catalogs.NewManager()
	symbol := "ETH-USDT"
	_, err := QuantityScaleForSymbol(m, &symbol)
	if err == nil {
		t.Fatal("expected error for unhydrated catalog")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("want unavailable error, got %v", err)
	}
	if err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "ETH-USDT",
				"symbol_id":           float64(2),
				"base_quantity_scale": float64(6),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	scale, err := QuantityScaleForSymbol(m, &symbol)
	if err != nil || scale != 6 {
		t.Fatalf("expected scale 6, got scale=%d err=%v", scale, err)
	}
}

func TestCreateOrderDecimalQtyNilSymbolValidationError(t *testing.T) {
	scale, err := QuantityScaleForSymbol(nil, nil)
	if err == nil {
		t.Fatal("expected QuantityScaleForSymbol error")
	}
	sid := uint32(1)
	tif := "gtc"
	price := models.PriceFromDecimal("50000")
	req := models.CreateOrderRequest{
		SymbolID: &sid, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("0.1"), Price: &price,
	}
	_, err = CreateOrderToProto(req, scale)
	if err == nil {
		t.Fatal("expected ValidationError for decimal qty without catalog+symbol scale")
	}
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}

func TestCreateOrderRejectsStrayPriceOnMarket(t *testing.T) {
	symbol := "BTC-USDT"
	price := models.PriceFromDecimal("65000")
	req := models.CreateOrderRequest{
		Symbol: &symbol, Side: "buy", OrderType: "market",
		Qty: models.QtyFromScaledInt(1_000_000), Price: &price,
	}
	_, err := CreateOrderToProto(req, 8)
	if err == nil {
		t.Fatal("expected ValidationError for market+price")
	}
	if !strings.Contains(err.Error(), "price is not valid for market") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestModifyOrderDecimalQtyNilSymbolValidationError(t *testing.T) {
	scale, err := QuantityScaleForSymbol(nil, nil)
	if err == nil {
		t.Fatal("expected QuantityScaleForSymbol error")
	}
	qty := models.QtyFromDecimal("0.25")
	_, err = ModifyOrderToProto("BTC-USD", models.OrderKeyByID("100"), nil, nil, nil, &qty, nil, nil, scale)
	if err == nil {
		t.Fatal("expected ValidationError for decimal qty without catalog+symbol scale")
	}
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}

func TestBatchCreateDecimalQtyNilSymbolValidationError(t *testing.T) {
	scale, err := QuantityScaleForSymbol(nil, nil)
	if err == nil {
		t.Fatal("expected QuantityScaleForSymbol error")
	}
	sid := uint32(1)
	tif := "gtc"
	price := models.PriceFromDecimal("100")
	items := []models.CreateOrderRequest{{
		SymbolID: &sid, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("1.5"), Price: &price,
	}}
	_, err = BatchCreateOrdersToProto(items, nil, nil, false, scale)
	if err == nil {
		t.Fatal("expected ValidationError for decimal qty without catalog+symbol scale")
	}
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}
