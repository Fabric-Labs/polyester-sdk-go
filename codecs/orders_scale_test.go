package codecs

import (
	"errors"
	"testing"

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

func TestModifyOrderDecimalQtyNilSymbolValidationError(t *testing.T) {
	scale, err := QuantityScaleForSymbol(nil, nil)
	if err == nil {
		t.Fatal("expected QuantityScaleForSymbol error")
	}
	oid := "100"
	qty := models.QtyFromDecimal("0.25")
	_, err = ModifyOrderToProto("BTC-USD", &oid, nil, nil, nil, nil, &qty, nil, nil, scale)
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
