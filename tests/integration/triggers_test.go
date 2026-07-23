//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestTriggersList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "triggers.list", func() (models.TriggersList, error) {
		return client.Triggers.List(ctx, nil, nil, nil, nil, 10, nil)
	})
	if result.Triggers == nil {
		t.Fatal("expected triggers list")
	}
}
