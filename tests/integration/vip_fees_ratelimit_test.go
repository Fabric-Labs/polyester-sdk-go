//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestListVIPTiersOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "vip.list_vip_tiers", func() (models.VIPTiersList, error) {
		return client.VIP.ListVIPTiers(ctx)
	})
	if len(result.Tiers) > 11 {
		t.Fatalf("tiers=%d want <= 11", len(result.Tiers))
	}
}

func TestGetVIPStatusOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "vip.get_vip_status", func() (models.VIPStatus, error) {
		return client.VIP.GetVIPStatus(ctx)
	})
	if result.Tier > 10 {
		t.Fatalf("tier=%d want 0-10", result.Tier)
	}
}

func TestGetSpotFeeRatesOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "fees.get_spot_fee_rates", func() (models.SpotFeeRatesList, error) {
		return client.Fees.GetSpotFeeRates(ctx, nil, nil, nil)
	})
}

func TestGetRateLimitConfigOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "rate_limits.get_rate_limit_config", func() (models.RateLimitConfig, error) {
		return client.RateLimits.GetRateLimitConfig(ctx)
	})
}

func TestGetTradingRateLimitsOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "rate_limits.get_trading_rate_limits", func() (models.TradingRateLimits, error) {
		return client.RateLimits.GetTradingRateLimits(ctx, nil, nil)
	})
}
