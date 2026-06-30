//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestMarketOverviewList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "market_overview.list", func() (models.MarketOverviewList, error) {
		return client.MarketOverview.List(ctx, nil, 10, "", false)
	})
	if result.Markets == nil {
		t.Fatal("expected markets list")
	}
	for _, m := range result.Markets {
		if m.Symbol == "" {
			t.Fatalf("market missing symbol: %+v", m)
		}
	}
}

func TestZipperDepositWithdrawConfig(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	cfg := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	if len(cfg.Assets) == 0 || len(cfg.Chains) == 0 {
		t.Fatalf("expected zipper config assets/chains: %+v", cfg)
	}
}
