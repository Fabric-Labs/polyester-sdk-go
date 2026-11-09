package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// marketRefPriceOrder is a BTC-USDT order with market_client_ref_price set.
func marketRefPriceOrder(side, orderType, tif string) models.CreateOrderRequest {
	ref := models.PriceFromDecimal("100")
	req := models.CreateOrderRequest{
		Symbol:               new("BTC-USDT"),
		Side:                 side,
		OrderType:            orderType,
		Qty:                  models.QtyFromDecimal("0.01"),
		MarketClientRefPrice: &ref,
	}
	if orderType == "limit" {
		price := models.PriceFromDecimal("100")
		req.Price = &price
		req.TIF = &tif
		if tif == "gtd" {
			req.ExpiresAt = new(time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		}
	}
	return req
}

// The public contract carries client_ref_price_ticks on MarketIoc only, so
// a limit order with market_client_ref_price is refused before sending
// instead of having the reference price silently dropped.
func TestLimitOrderRefusesMarketClientRefPriceBeforeSending(t *testing.T) {
	for _, tif := range []string{"gtc", "gtd", "ioc", "fok"} {
		for _, side := range []string{"buy", "sell"} {
			for _, method := range []string{"Create", "Preview", "BatchCreate"} {
				t.Run(tif+"/"+side+"/"+method, func(t *testing.T) {
					factory, captures := scopeCaptureFactory(t)
					orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
					req := marketRefPriceOrder(side, "limit", tif)
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
					if !strings.Contains(err.Error(), "market_client_ref_price is only valid for market orders") {
						t.Fatalf("err = %v, want the market_client_ref_price refusal", err)
					}
				})
			}
		}
	}
}

func TestMarketOrderSendsMarketClientRefPrice(t *testing.T) {
	for _, side := range []string{"buy", "sell"} {
		t.Run(side, func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
			_, _ = orders.Create(context.Background(), marketRefPriceOrder(side, "market", ""), nil)
			if len(captures) != 1 {
				t.Fatal("market order with market_client_ref_price never reached transport")
			}
			got := capturedField(<-captures, "order", "market_ioc", "client_ref_price_ticks").Int()
			if got != 100_000_000_000 {
				t.Fatalf("client_ref_price_ticks = %d, want 100000000000", got)
			}
		})
	}
}
