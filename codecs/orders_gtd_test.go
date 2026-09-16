package codecs

import (
	"strings"
	"testing"
	"time"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderIntentEncodesLimitGTD(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	tif := "gtd"
	price := models.PriceFromTicksInt(50_000_000_000)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)
	intent, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol:    &symbol,
		SymbolID:  &sid,
		Side:      "buy",
		OrderType: "limit",
		TIF:       &tif,
		Qty:       models.QtyFromScaledInt(10_000_000),
		Price:     &price,
		PostOnly:  true,
		ExpiresAt: &expires,
	}, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	gtd := intent.GetLimitGtd()
	if gtd == nil {
		t.Fatalf("expected limit_gtd, got %+v", intent)
	}
	if gtd.GetPriceTicks() != 50_000_000_000 || !gtd.GetPostOnly() {
		t.Fatalf("gtd=%+v", gtd)
	}
	if FormatExpireAt(gtd.GetExpireAt()) != expires {
		t.Fatalf("expire_at=%q want %q", FormatExpireAt(gtd.GetExpireAt()), expires)
	}
}

func TestOrderIntentGTDRequiresExpiresAt(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	tif := "gtd"
	price := models.PriceFromTicksInt(50_000_000_000)
	_, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol:    &symbol,
		SymbolID:  &sid,
		Side:      "buy",
		OrderType: "limit",
		TIF:       &tif,
		Qty:       models.QtyFromScaledInt(10_000_000),
		Price:     &price,
	}, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "requires expires_at") {
		t.Fatalf("err=%v", err)
	}
}

func TestOrderIntentRejectsExpiresAtOutsideGTD(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	tif := "gtc"
	price := models.PriceFromTicksInt(50_000_000_000)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol:    &symbol,
		SymbolID:  &sid,
		Side:      "buy",
		OrderType: "limit",
		TIF:       &tif,
		Qty:       models.QtyFromScaledInt(10_000_000),
		Price:     &price,
		ExpiresAt: &expires,
	}, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "only valid for limit GTD") {
		t.Fatalf("err=%v", err)
	}
}

func TestOrderIntentRejectsGTDWindow(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	tif := "gtd"
	price := models.PriceFromTicksInt(50_000_000_000)
	expires := time.Now().UTC().Add(31 * 24 * time.Hour).Format(time.RFC3339)
	_, err := OrderIntentToProto(models.CreateOrderRequest{
		Symbol:    &symbol,
		SymbolID:  &sid,
		Side:      "buy",
		OrderType: "limit",
		TIF:       &tif,
		Qty:       models.QtyFromScaledInt(10_000_000),
		Price:     &price,
		ExpiresAt: &expires,
	}, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "1 second and 30 days") {
		t.Fatalf("err=%v", err)
	}
}

func TestTimeInForceNameGTD(t *testing.T) {
	if TimeInForceName(orderv1.TimeInForce_GTD) != "gtd" {
		t.Fatal(TimeInForceName(orderv1.TimeInForce_GTD))
	}
}
