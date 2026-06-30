//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestTransfersList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallRequired(t, "transfers.list", func() (models.TransfersList, error) {
		return client.Transfers.List(ctx, nil, nil, 5, false, nil)
	})
}
