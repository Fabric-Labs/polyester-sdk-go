//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

const usdE8 = 100_000_000

func TestCurrencyConversionConfig(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "market_overview.get_currency_conversion_config", func() (models.CurrencyConversionConfig, error) {
		return client.MarketOverview.GetCurrencyConversionConfig(ctx)
	})
	for _, item := range result.Fiat {
		if item.Code == "" {
			t.Fatalf("fiat missing code: %+v", item)
		}
	}
	for _, item := range result.Stablecoins {
		if item.Code == "" {
			t.Fatalf("stablecoin missing code: %+v", item)
		}
	}
}

func TestCurrencyConversionRates(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "market_overview.get_currency_conversion_rates", func() (models.CurrencyConversionRates, error) {
		return client.MarketOverview.GetCurrencyConversionRates(ctx)
	})
	if result.Fiat != nil {
		for _, rate := range result.Fiat.Rates {
			if rate.Code == "" || rate.UnitsPerUsdE8 == 0 {
				t.Fatalf("fiat rate missing identity: %+v", rate)
			}
			if rate.Code == "USD" && rate.UnitsPerUsdE8 != usdE8 {
				t.Fatalf("USD units_per_usd_e8=%d want %d", rate.UnitsPerUsdE8, usdE8)
			}
		}
	}
	for _, rate := range result.Stablecoins {
		if rate.Code == "" || rate.UsdPerUnitE8 == 0 {
			t.Fatalf("stablecoin rate missing identity: %+v", rate)
		}
	}
}
