//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestUserTradesList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "trades.list", func() (models.UserTradesList, error) {
		return client.Trades.List(ctx, nil, nil, &symbol, nil, 5, nil)
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
		if testutil.NonNegativeIntStringPositive(t, trade.PriceTicks).Sign() == 0 {
			t.Fatalf("trade price_ticks: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, trade.QtyScaled).Sign() == 0 {
			t.Fatalf("trade qty_scaled: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, trade.TsNs).Sign() == 0 {
			t.Fatalf("trade ts_ns: %+v", trade)
		}
	}
}
