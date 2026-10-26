package codecs

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestConditionalChildRejectsPostOnlyOutsideLimitGTC(t *testing.T) {
	price := models.PriceFromDecimal("60000")
	cases := []struct {
		orderType string
		tif       string
	}{
		{"market", ""},
		{"limit", "ioc"},
		{"limit", "fok"},
	}
	for _, tc := range cases {
		_, err := conditionalChildToProto(tc.orderType, tc.tif, &price, "BTC-USDT", true, orderv1.Side_SELL, nil, nil)
		if err == nil {
			t.Fatalf("%s/%s: expected validation error", tc.orderType, tc.tif)
		}
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s/%s: want ValidationError, got %T %v", tc.orderType, tc.tif, err, err)
		}
	}
	_, err := conditionalChildToProto("limit", "gtc", &price, "BTC-USDT", true, orderv1.Side_SELL, nil, nil)
	if err != nil {
		t.Fatalf("limit GTC post_only should be allowed: %v", err)
	}
}

func createTrailing(side string, limitPrice, activationPrice *models.PriceInput) (*triggersv1.CreateTriggerRequest, error) {
	distance := int32(100)
	return CreateTriggerToProto(
		1, "BTC-USDT", "trailing_stop", nil, side, models.QtyFromDecimal("0.1"),
		"market", limitPrice, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceBps: &distance, ActivationPrice: activationPrice},
	)
}

func requireValidationError(t *testing.T, err error, contains string) {
	t.Helper()
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
	if !strings.Contains(validationErr.Msg, contains) {
		t.Fatalf("error %q does not mention %q", validationErr.Msg, contains)
	}
}

func TestTrailingStopEncodesBuyMaximumPrice(t *testing.T) {
	limit := models.PriceFromDecimal("65000.123456789")
	activation := models.PriceFromDecimal("64000")
	req, err := createTrailing("buy", &limit, &activation)
	if err != nil {
		t.Fatal(err)
	}
	trailing := req.GetTrigger().GetTrailingStop()
	if trailing.GetSide() != orderv1.Side_BUY {
		t.Fatalf("side=%v", trailing.GetSide())
	}
	if trailing.GetLimitPriceTicks() != 65_000_123_456_789 {
		t.Fatalf("limit_price_ticks=%d, want 65000123456789 (9 decimals)", trailing.GetLimitPriceTicks())
	}
	if trailing.GetActivationPriceTicks() != 64_000_000_000_000 {
		t.Fatalf("activation_price_ticks=%d", trailing.GetActivationPriceTicks())
	}

	ticks := models.PriceFromTicksInt(70_000_000_000_000)
	req, err = createTrailing("BUY", &ticks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.GetTrigger().GetTrailingStop().GetLimitPriceTicks(); got != 70_000_000_000_000 {
		t.Fatalf("limit_price_ticks=%d from raw ticks", got)
	}
}

func TestTrailingStopBuyRequiresMaximumPrice(t *testing.T) {
	_, err := createTrailing("buy", nil, nil)
	requireValidationError(t, err, "requires limit_price")
}

func TestTrailingStopSellRejectsMaximumPrice(t *testing.T) {
	limit := models.PriceFromDecimal("65000")
	_, err := createTrailing("sell", &limit, nil)
	requireValidationError(t, err, "side=sell does not accept limit_price")
}

func TestTrailingStopBuyRejectsInvalidMaximumPrice(t *testing.T) {
	zero := models.PriceFromTicksInt(0)
	_, err := createTrailing("buy", &zero, nil)
	requireValidationError(t, err, "limit_price must be positive")

	limit := models.PriceFromDecimal("64000")
	activation := models.PriceFromDecimal("64000.000000001")
	_, err = createTrailing("buy", &limit, &activation)
	requireValidationError(t, err, "at least activation_price")
}

func TestTrailingStopEncodesSellSide(t *testing.T) {
	distance := int32(100)
	req, err := CreateTriggerToProto(
		1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceBps: &distance},
	)
	if err != nil {
		t.Fatal(err)
	}
	if req.GetTrigger().GetSymbolId() != 1 {
		t.Fatalf("symbol_id=%d", req.GetTrigger().GetSymbolId())
	}
	trailing := req.GetTrigger().GetTrailingStop()
	if trailing == nil {
		t.Fatal("trailing_stop strategy missing")
	}
	if trailing.GetSide() != orderv1.Side_SELL {
		t.Fatalf("side=%v", trailing.GetSide())
	}
	if trailing.GetTrailingDistanceBps() != 100 {
		t.Fatalf("distance=%v", trailing.GetTrailingDistanceBps())
	}
	if trailing.GetLimitPriceTicks() != 0 {
		t.Fatalf("sell limit_price_ticks=%d, want unset", trailing.GetLimitPriceTicks())
	}
}
