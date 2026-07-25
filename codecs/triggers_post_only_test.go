package codecs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
		_, err := conditionalChildToProto(tc.orderType, tc.tif, &price, "BTC-USDT", true)
		if err == nil {
			t.Fatalf("%s/%s: expected validation error", tc.orderType, tc.tif)
		}
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s/%s: want ValidationError, got %T %v", tc.orderType, tc.tif, err, err)
		}
	}
	_, err := conditionalChildToProto("limit", "gtc", &price, "BTC-USDT", true)
	if err != nil {
		t.Fatalf("limit GTC post_only should be allowed: %v", err)
	}
}

func TestTrailingStopRejectsBuy(t *testing.T) {
	distance := int32(100)
	_, err := CreateTriggerToProto(
		"BTC-USDT", "trailing_stop", nil, "buy", models.QtyFromDecimal("0.1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceBps: &distance},
	)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}
