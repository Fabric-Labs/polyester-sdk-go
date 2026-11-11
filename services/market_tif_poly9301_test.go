package services

import (
	"context"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// POLY-9301: the public contract has one market execution, MarketIoc
// ("Market orders always execute as immediate-or-cancel"). A market order
// that names any other time in force is refused before sending, as gtd
// already was, instead of being sent as IOC.
func TestPoly9301MarketOrderRefusesNonIOCTIFBeforeSending(t *testing.T) {
	for _, tif := range []string{"fok", "gtc", "gtd", "FOK"} {
		for _, side := range []string{"buy", "sell"} {
			for _, method := range []string{"Create", "Preview", "BatchCreate"} {
				t.Run(tif+"/"+side+"/"+method, func(t *testing.T) {
					factory, captures := scopeCaptureFactory(t)
					orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
					req := models.CreateOrderRequest{
						Symbol:    new("BTC-USDT"),
						Side:      side,
						OrderType: "market",
						TIF:       new(tif),
						Qty:       models.QtyFromDecimal("0.001"),
					}
					var err error
					switch method {
					case "Create":
						_, err = orders.Create(context.Background(), req, nil)
					case "Preview":
						_, err = orders.Preview(context.Background(), req, nil)
					case "BatchCreate":
						_, err = orders.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{req}, nil, nil, nil, false)
					}
					requireRefusedBeforeTransport(t, err, captures)
					want := "tif='" + strings.ToLower(tif) + "' is only valid for limit orders"
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("err = %v, want %q", err, want)
					}
				})
			}
		}
	}
}

func TestPoly9301MarketOrderSendsIOCWhenTIFIsIOCOrUnset(t *testing.T) {
	for _, tif := range []string{"", "ioc", "IOC"} {
		for _, side := range []string{"buy", "sell"} {
			t.Run(tif+"/"+side, func(t *testing.T) {
				factory, captures := scopeCaptureFactory(t)
				orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
				req := models.CreateOrderRequest{
					Symbol:    new("BTC-USDT"),
					Side:      side,
					OrderType: "market",
					Qty:       models.QtyFromDecimal("0.001"),
				}
				if tif != "" {
					req.TIF = new(tif)
				}
				_, _ = orders.Create(context.Background(), req, nil)
				if len(captures) != 1 {
					t.Fatal("market order never reached transport")
				}
				if !capturedField(<-captures, "order", "market_ioc").Message().IsValid() {
					t.Fatal("market order was not sent as market_ioc")
				}
			})
		}
	}
}
