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

	result := testutil.CallRequired(t, "transfers.list", func() (models.TransfersList, error) {
		return client.Transfers.List(ctx, nil, nil, 5, false, nil)
	})
	for _, transfer := range result.Transfers {
		for _, side := range []*models.TransferSide{transfer.Source, transfer.Destination} {
			if side == nil {
				continue
			}
			if side.Kind == "external_address" && side.ChainID != nil && *side.ChainID == 0 {
				t.Fatalf("external zipper chain_id must not be the zero sentinel: %+v", side)
			}
		}
	}
}
