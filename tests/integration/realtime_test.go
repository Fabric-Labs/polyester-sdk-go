//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

const realtimeHeartbeatHold = 35 * time.Second

func TestPublicTradesSubscriptionSurvivesCentrifugoPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long realtime heartbeat in short mode")
	}
	client, ok, err := testutil.LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), realtimeHeartbeatHold+20*time.Second)
	defer cancel()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	sub, err := client.MarketData.SubscribeTrades(ctx, &symbol, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	deadline := time.Now().Add(realtimeHeartbeatHold)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-sub.Done():
			t.Fatalf("public trades subscription closed before Centrifugo heartbeat window elapsed (%s)", realtimeHeartbeatHold)
		case _, ok := <-sub.Messages():
			if !ok {
				t.Fatalf("public trades subscription closed before Centrifugo heartbeat window elapsed (%s)", realtimeHeartbeatHold)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func TestOrdersSubscribeReceivesConnectionOptional(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping realtime subscription in short mode")
	}
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()
	if client.DefaultAccountID == nil || *client.DefaultAccountID == "" {
		t.Skip("POLYESTER_ACCOUNT_ID required for private orders realtime")
	}

	sub, err := client.Orders.Subscribe(ctx, *client.DefaultAccountID)
	if err != nil {
		if testutil.RouteUnavailable(err) {
			t.Skipf("orders.subscribe not mounted on devnet: %v", err)
		}
		if testutil.JWTSessionOnly(err) {
			t.Skipf("orders.subscribe requires JWT/session auth: %v", err)
		}
		t.Fatal(err)
	}
	defer sub.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	select {
	case <-waitCtx.Done():
	case <-sub.Done():
		t.Skip("orders.subscribe closed without publications (no order activity)")
	case _, ok := <-sub.Messages():
		if !ok {
			t.Skip("orders.subscribe closed without publications (no order activity)")
		}
	case <-time.After(5 * time.Second):
	}
}
