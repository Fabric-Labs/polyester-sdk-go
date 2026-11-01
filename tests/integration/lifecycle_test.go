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
