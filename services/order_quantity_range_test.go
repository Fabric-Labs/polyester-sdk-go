package services

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// orderWriteSentQty sends one Create or Preview for qty on market id and
// returns the base quantity the server received, or the refusal.
func orderWriteSentQty(t *testing.T, wire, method string, pairs []any, id uint32, qty models.QtyInput) (int64, bool, error) {
	t.Helper()
	h := &readPreservationHandler{}
	factory, cats := newReadPreservationServices(t, h, wire)
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": pairs}); err != nil {
		t.Fatal(err)
	}
	service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
	price := models.PriceFromDecimal("1")
	req := models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "limit", Price: &price, Qty: qty}
	var err error
	if method == "create" {
		_, err = service.Create(context.Background(), req, nil)
	} else {
		_, err = service.Preview(context.Background(), req, nil)
	}
	switch len(h.calls) {
	case 0:
		return 0, false, err
	case 1:
	default:
		t.Fatalf("%d requests sent", len(h.calls))
	}
	switch msg := h.calls[0].(type) {
	case *orders.CreateOrderRequest:
		return msg.Order.GetBaseQtyScaled(), true, err
	case *orders.PreviewOrderRequest:
		return msg.Order.GetBaseQtyScaled(), true, err
	}
	t.Fatalf("unexpected request %T", h.calls[0])
	return 0, false, nil
}

func requireRefusedBeforeSending(t *testing.T, sent bool, got int64, err error, want string) {
	t.Helper()
	if sent {
		t.Fatalf("sent base quantity %d, want refusal %q", got, want)
	}
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), want) {
		t.Fatalf("want ValidationError containing %q, got %T %v", want, err, err)
	}
}

func TestOrderWriteRefusesDecimalQuantityBeyondInt64AtScale18(t *testing.T) {
	pairs := []any{map[string]any{"symbol": "WIDE-USDT", "symbol_id": float64(7), "base_quantity_scale": float64(18), "quote_quantity_scale": float64(6)}}
	for _, wire := range []string{"binary", "json"} {
		for _, method := range []string{"create", "preview"} {
			t.Run(wire+"/"+method, func(t *testing.T) {
				for qty, want := range map[string]int64{
					"1":                    1_000_000_000_000_000_000,
					"9.223372036854775807": math.MaxInt64,
				} {
					got, sent, err := orderWriteSentQty(t, wire, method, pairs, 7, models.QtyFromDecimal(qty))
					if err != nil || !sent || got != want {
						t.Fatalf("qty %s: sent=%v got %d want %d err=%v", qty, sent, got, want, err)
					}
				}
				for qty, want := range map[string]string{
					"10":                    "qty exceeds int64 range",
					"9.223372036854775808":  "qty exceeds int64 range",
					"18.446744073709551615": "qty exceeds int64 range",
					"18.446744073709551616": "qty out of range",
				} {
					got, sent, err := orderWriteSentQty(t, wire, method, pairs, 7, models.QtyFromDecimal(qty))
					requireRefusedBeforeSending(t, sent, got, err, want)
				}
			})
		}
	}
}

func TestOrderWriteDecimalQuantityAtKnownZeroScale(t *testing.T) {
	pairs := []any{
		map[string]any{"symbol": "BONK-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(0), "quote_quantity_scale": float64(6)},
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)},
	}
	for _, wire := range []string{"binary", "json"} {
		for _, method := range []string{"create", "preview"} {
			t.Run(wire+"/"+method, func(t *testing.T) {
				for _, qty := range []models.QtyInput{
					models.QtyFromDecimal("1000000"),
					models.QtyFromDecimal("1000000.000"),
					models.QtyFromScaled(models.MustQtyScaled(1000000).WithScale(0)),
				} {
					got, sent, err := orderWriteSentQty(t, wire, method, pairs, 5, qty)
					if err != nil || !sent || got != 1000000 {
						t.Fatalf("BONK %+v: sent=%v got %d err=%v", qty, sent, got, err)
					}
				}
				got, sent, err := orderWriteSentQty(t, wire, method, pairs, 5, models.QtyFromDecimal("1000000.5"))
				requireRefusedBeforeSending(t, sent, got, err, "qty supports at most 0 decimal places: 1000000.5")
				// BTC control at scale 8.
				got, sent, err = orderWriteSentQty(t, wire, method, pairs, 1, models.QtyFromDecimal("0.00016"))
				if err != nil || !sent || got != 16000 {
					t.Fatalf("BTC: sent=%v got %d err=%v", sent, got, err)
				}
				// A market missing from the catalog is still refused.
				got, sent, err = orderWriteSentQty(t, wire, method, pairs, 99, models.QtyFromDecimal("1000000"))
				if sent || err == nil {
					t.Fatalf("unknown market: sent=%v got %d err=%v", sent, got, err)
				}
			})
		}
	}
}
