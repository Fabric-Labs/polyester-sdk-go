//go:build integration

package integration_test

import (
	"strconv"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestAuthMeReturnsIdentityFields(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "auth.me", func() (models.MeResult, error) {
		return client.Auth.Me(ctx)
	})
	if result.AccountID == "" && result.APIKeyID == "" {
		t.Fatal("me() should identify the caller")
	}
}

func TestChainAnalyticsUnifiedBalancesSeriesShapeOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	zipper := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	if len(zipper.Assets) == 0 {
		t.Skip("zipper config missing assets")
	}
	assetID := zipper.Assets[0].LedgerID
	if assetID == 0 {
		t.Skip("cannot resolve asset id for chain analytics")
	}

	result := testutil.CallOptional(t, "chain_analytics.get_unified_asset_balances", func() (models.ApiData, error) {
		return client.ChainAnalytics.GetUnifiedAssetBalances(ctx, assetID, "7d", "", 0, 0)
	})
	testutil.AssertAPIDataShape(t, result.Raw, "range", "points", "start_ts_sec", "end_ts_sec")
	points, err := strconv.Atoi(anyString(result.Raw["points"]))
	if err != nil {
		t.Fatalf("parse points: %v", err)
	}
	if points < 0 {
		t.Fatalf("points=%d", points)
	}
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}
