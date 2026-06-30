//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestSpotConfigSmoke(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	cfg := testutil.CallRequired(t, "market_data.get_spot_config", func() (models.SpotConfig, error) {
		return client.MarketData.GetSpotConfig(ctx)
	})
	if len(cfg.Raw) == 0 {
		t.Fatal("expected spot config payload")
	}
}
