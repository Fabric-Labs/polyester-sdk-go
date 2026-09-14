//go:build integration

package integration_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestUserTradesListAcceptsTransfersAndScopeFields(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	dummyID := codecs.FormatUint64ID(1)
	result, err := client.Trades.List(ctx, nil, nil, nil, nil, 5, nil, nil, models.ListUserTradesOptions{
		OrderID:          &dummyID,
		IncludeTransfers: true,
	})
	if err != nil && !testutil.RouteUnavailable(err) {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "not found") {
			t.Fatalf("trades.list rejected new fields: %v", err)
		}
		return
	}
	if err == nil && result.Transfers == nil {
		result.Transfers = []models.OrderTransfer{}
	}
}

func TestUserTradesList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "trades.list", func() (models.UserTradesList, error) {
		return client.Trades.List(ctx, nil, nil, &symbol, nil, 5, nil, nil)
	})
	if result.Trades == nil {
		t.Fatal("expected trades list")
	}
	for _, trade := range result.Trades {
		if trade.SymbolID == 0 || trade.MatchID == "" || trade.OrderID == "" {
			t.Fatalf("trade missing ids: %+v", trade)
		}
		if trade.Side != "buy" && trade.Side != "sell" {
			t.Fatalf("trade side=%q", trade.Side)
		}
		if testutil.NonNegativeIntStringPositive(t, fmt.Sprint(trade.Price.Ticks())).Sign() == 0 {
			t.Fatalf("trade price_ticks: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, fmt.Sprint(trade.Qty.Scaled())).Sign() == 0 {
			t.Fatalf("trade qty_scaled: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, trade.TsNs).Sign() == 0 {
			t.Fatalf("trade ts_ns: %+v", trade)
		}
		if trade.FeeAmountE18 == "" {
			t.Fatalf("trade fee_amount_e18 missing: %+v", trade)
		}
		_ = testutil.NonNegativeIntString(t, trade.FeeAmountE18)
		if trade.ReferralShareAmountE18 == "" {
			t.Fatalf("trade referral_share_amount_e18 missing: %+v", trade)
		}
		_ = testutil.NonNegativeIntString(t, trade.ReferralShareAmountE18)
		if trade.FeeAsset != "" && trade.FeeAsset != "quote" && trade.FeeAsset != "base" &&
			!strings.HasPrefix(trade.FeeAsset, "unknown(") {
			t.Fatalf("trade fee_asset=%q", trade.FeeAsset)
		}
	}
}

func TestUserTradesListOrderAndLineageFilters(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	listed := testutil.CallRequired(t, "trades.list", func() (models.UserTradesList, error) {
		return client.Trades.List(ctx, nil, nil, &symbol, nil, 5, nil, nil, models.ListUserTradesOptions{
			IncludeTransfers: true,
		})
	})
	if len(listed.Trades) == 0 {
		listed = testutil.CallRequired(t, "trades.list_unfiltered", func() (models.UserTradesList, error) {
			return client.Trades.List(ctx, nil, nil, nil, nil, 5, nil, nil, models.ListUserTradesOptions{
				IncludeTransfers: true,
			})
		})
	}
	if len(listed.Trades) == 0 {
		t.Skip("no user trades on devnet; cannot exercise lineage filters")
	}
	sample := listed.Trades[0]
	byOrder := testutil.CallRequired(t, "trades.list_order", func() (models.UserTradesList, error) {
		return client.Trades.List(ctx, nil, nil, nil, nil, 5, nil, nil, models.ListUserTradesOptions{
			OrderID:          &sample.OrderID,
			IncludeTransfers: true,
		})
	})
	for _, trade := range byOrder.Trades {
		if trade.OrderID != sample.OrderID {
			t.Fatalf("order-scoped trade=%+v sample=%s", trade, sample.OrderID)
		}
	}
	if sample.Lineage == nil || sample.Lineage.ID == "" {
		return
	}
	gen := sample.Lineage.Generation
	byLineage := testutil.CallRequired(t, "trades.list_lineage", func() (models.UserTradesList, error) {
		return client.Trades.List(ctx, nil, nil, nil, nil, 5, nil, nil, models.ListUserTradesOptions{
			LineageID:         &sample.Lineage.ID,
			ThroughGeneration: &gen,
			IncludeTransfers:  true,
		})
	})
	for _, trade := range byLineage.Trades {
		if trade.Lineage != nil && (trade.Lineage.ID != sample.Lineage.ID || trade.Lineage.Generation > gen) {
			t.Fatalf("lineage-scoped trade=%+v", trade)
		}
	}
}
