//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

// Policy unary RPCs are JWT/session-only; API-key coverage is subscribe-only
// (see TestPrivateAuthAndLedgerSubscribeConnects).
func TestPoliciesSubscribeAPIPoliciesOptional(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping realtime subscription in short mode")
	}
	client, ok, err := testutil.LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()
	if client.DefaultAccountID == nil || *client.DefaultAccountID == "" {
		t.Skip("POLYESTER_ACCOUNT_ID required for private realtime")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sub, err := client.Policies.SubscribeAPIPolicies(ctx, *client.DefaultAccountID)
	if err != nil {
		if testutil.RouteUnavailable(err) || testutil.JWTSessionOnly(err) || testutil.DevnetUnavailable(err) || testutil.DevnetProtoMismatch(err) {
			t.Skipf("policies.subscribe_api_policies unavailable: %v", err)
		}
		t.Fatalf("policies.subscribe_api_policies: %v", err)
	}
	defer sub.Close()

	select {
	case <-sub.Done():
		t.Skip("policies.subscribe_api_policies closed without publications")
	case <-sub.Messages():
	case <-time.After(5 * time.Second):
	}
}
