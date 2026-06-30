//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderbookGetOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallOptional(t, "orderbook.get", func() (models.OrderbookData, error) {
		return client.Orderbook.Get(ctx, symbol, 5)
	})
	if result.Symbol == "" {
		t.Fatalf("expected orderbook symbol: %+v", result)
	}
}
