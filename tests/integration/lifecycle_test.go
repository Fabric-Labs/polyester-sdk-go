//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestLifecycleListFlowsOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "lifecycle.list_flows", func() (models.LifecycleFlowsList, error) {
		return client.Lifecycle.ListFlows(ctx, 5, nil, nil, nil, nil, false)
	})
	if result.Flows == nil {
		t.Fatal("expected flows list")
	}
}

func TestGuardSignerGetStatusOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "guard_signer.get_status", func() (*models.GuardSignerStatus, error) {
		return client.GuardSigner.GetStatus(ctx, nil, nil)
	})
}

func TestLayoutGetLayoutsOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	data := testutil.CallOptional(t, "layout.get_layouts", func() (models.ApiData, error) {
		return client.Layout.GetLayouts(ctx, 5, "")
	})
	if len(data.Raw) == 0 {
		t.Skip("layout service returned empty payload on devnet")
	}
}

func TestPolychartGetMarketLayersOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "polychart.get_market_layers", func() (models.ApiData, error) {
		return client.Polychart.GetMarketLayers(ctx, 1)
	})
}
