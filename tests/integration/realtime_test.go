//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
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

	// Heartbeat / mutation paths honor POLYESTER_TEST_TRADE_SYMBOL (F-23).
	symbol := testutil.TradeSymbol(t, client, ctx)
	t.Logf("heartbeat trade_symbol=%s", symbol)
	sub, err := client.MarketData.SubscribeTrades(ctx, &symbol, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	deadline := time.Now().Add(realtimeHeartbeatHold)
	publications := 0
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
			publications++
		case <-time.After(2 * time.Second):
		}
	}
	if publications == 0 {
		t.Fatal("protobuf subscription stayed open but delivered no publications")
	}
}

func TestOrdersSubscribeReceivesSnapshot(t *testing.T) {
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

	// The private order stream confirms the subscription, then sends a snapshot.
	for {
		select {
		case <-waitCtx.Done():
			t.Fatal("orders.subscribe sent no snapshot within 8s")
		case event, ok := <-sub.Messages():
			if !ok {
				if testutil.IsPermissionDenied(sub.Err()) {
					testutil.SoftSkipf(t, "orders.subscribe permission denied: %v", sub.Err())
				}
				t.Fatalf("orders.subscribe ended: %v", sub.Err())
			}
			if _, ok := event.(realtime.OrderSnapshot); ok {
				return
			}
		}
	}
}
