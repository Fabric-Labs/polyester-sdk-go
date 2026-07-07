//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestBalancesList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	for _, row := range result.Balances {
		if row.AssetID == 0 {
			t.Fatalf("balance missing asset_id: %+v", row)
		}
		testutil.NonNegativeIntString(t, row.Trading)
		testutil.NonNegativeIntString(t, row.Funding)
		testutil.NonNegativeIntString(t, row.Reserved)
		testutil.NonNegativeIntString(t, row.Available)
	}
}

func TestBalancesGetHealth(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "balances.get_health", func() (models.LedgerHealth, error) {
		return client.Balances.GetHealth(ctx)
	})
	if !result.OK {
		t.Fatalf("expected healthy ledger, got %+v", result)
	}
}

func TestBalancesGetBalanceHistory(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "balances.get_balance_history", func() (models.BalanceHistory, error) {
		return client.Balances.GetBalanceHistory(ctx, nil, "7d", nil, 0, nil)
	})
	if result.Range != "7d" {
		t.Fatalf("range=%q want 7d", result.Range)
	}
	if result.StartTsSec > result.EndTsSec {
		t.Fatalf("start_ts_sec=%d end_ts_sec=%d", result.StartTsSec, result.EndTsSec)
	}
	if result.Points < 0 {
		t.Fatalf("points=%d", result.Points)
	}
	for _, series := range result.Series {
		if len(series.BalanceQ) > result.Points {
			t.Fatalf("series points=%d exceed response points=%d", len(series.BalanceQ), result.Points)
		}
	}
}

func TestBalancesListHoldsOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "balances.list_holds", func() (models.HoldsList, error) {
		return client.Balances.ListHolds(ctx, nil, nil, 5, false)
	})
	if result.Holds == nil {
		t.Fatal("expected holds list")
	}
}
