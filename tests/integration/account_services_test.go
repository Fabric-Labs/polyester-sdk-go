//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestAPIKeysListReturnsKeySummaries(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "api_keys.list", func() (models.ApiKeysList, error) {
		return client.APIKeys.List(ctx, nil, nil)
	})
	for _, key := range result.Keys {
		if key.KeyID == "" || key.Status == "" {
			t.Fatalf("api key missing fields: %+v", key)
		}
	}
}

func TestAPIKeysGetRoundTripsListedKey(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "api_keys.list", func() (models.ApiKeysList, error) {
		return client.APIKeys.List(ctx, nil, nil)
	})
	if len(listed.Keys) == 0 {
		t.Skip("no API keys on devnet account")
	}
	keyID := listed.Keys[0].KeyID
	fetched := testutil.CallRequired(t, "api_keys.get", func() (*models.ApiKeySummary, error) {
		return client.APIKeys.Get(ctx, keyID)
	})
	if fetched == nil || fetched.KeyID != keyID {
		t.Fatalf("fetched=%+v want key_id=%s", fetched, keyID)
	}
	if fetched.Label != listed.Keys[0].Label {
		t.Fatalf("label=%q want %q", fetched.Label, listed.Keys[0].Label)
	}
}

func TestDepositListAddressesOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "deposit.list_addresses", func() (models.DepositAddressesList, error) {
		return client.Deposit.ListAddresses(ctx, 1, nil, nil)
	})
	for _, row := range result.Addresses {
		if row.ChainID == 0 || len(strings.TrimSpace(row.DepositAddress)) < 8 {
			t.Fatalf("address row invalid: %+v", row)
		}
	}
}
