//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestHeatmapGet(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallOptional(t, "heatmap.get", func() (models.ApiData, error) {
		return client.Heatmap.Get(ctx, &symbol, nil, "1s", 50, 100, "close", nil, nil, nil)
	})
	if result.Raw == nil {
		t.Fatal("expected heatmap raw payload")
	}
}
