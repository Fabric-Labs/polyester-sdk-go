//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrdersListOpen(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	limit := 10
	result := testutil.CallRequired(t, "orders.list_open", func() (models.OrdersList, error) {
		return client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
	})
	if result.Orders == nil {
		t.Fatal("expected orders list")
	}
	for _, order := range result.Orders {
		if order.OrderID == "" || order.SymbolID == 0 || order.Status == "" {
			t.Fatalf("order missing fields: %+v", order)
		}
	}
}

func TestOrdersGetRoundTripsListOpen(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	limit := 10
	listed := testutil.CallRequired(t, "orders.list_open", func() (models.OrdersList, error) {
		return client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
	})
	if len(listed.Orders) == 0 {
		t.Skip("no open orders on devnet; cannot round-trip orders.get")
	}
	sample := listed.Orders[0]
	byOrderID := testutil.CallRequired(t, "orders.get", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByID(sample.OrderID), nil, false, false)
	})
	if byOrderID.Order == nil || byOrderID.Order.OrderID != sample.OrderID {
		t.Fatalf("get by order_id=%+v sample=%+v", byOrderID.Order, sample)
	}
	if sample.ClientOrderID != "" {
		byClientID := testutil.CallRequired(t, "orders.get", func() (models.GetOrderResult, error) {
			return client.Orders.Get(ctx, nil, models.OrderKeyByClientID(sample.ClientOrderID), nil, false, false)
		})
		if byClientID.Order == nil || byClientID.Order.ClientOrderID != sample.ClientOrderID {
			t.Fatalf("get by client_order_id=%+v", byClientID.Order)
		}
	}
}

func TestOrdersListHistory(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "orders.list_history", func() (models.OrdersList, error) {
		return client.Orders.ListHistory(ctx, nil, nil, &symbol, nil, nil, 5, false, false)
	})
	for _, order := range result.Orders {
		if order.OrderID == "" || order.SymbolID == 0 || order.Status == "" {
			t.Fatalf("order missing fields: %+v", order)
		}
	}
}

func TestOrdersCancelAllDryRun(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallOptional(t, "orders.cancel_all", func() (models.CancelAllOrdersResult, error) {
		return client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, true, nil)
	})
	if result.Status == "" {
		t.Fatalf("expected status: %+v", result)
	}
	if result.MatchedOrders < 0 {
		t.Fatalf("matched_orders=%d", result.MatchedOrders)
	}
	if result.SubmittedCancels != 0 {
		t.Fatalf("submitted_cancels=%d want 0 for dry_run", result.SubmittedCancels)
	}
}
