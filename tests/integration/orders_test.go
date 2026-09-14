//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrdersGetAcceptsExecutionHistoryFlags(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	include := false
	dummyID := codecs.FormatUint64ID(1)
	_, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(dummyID), nil, false, false, models.GetOrderOptions{
		IncludeExecutionHistory: &include,
	})
	if err != nil && !testutil.RouteUnavailable(err) {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "not found") {
			t.Fatalf("state-only get rejected new fields: %v", err)
		}
	}
	include = true
	limit := uint32(5)
	result, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(dummyID), nil, false, false, models.GetOrderOptions{
		IncludeExecutionHistory: &include,
		Limit:                   &limit,
	})
	if err != nil && !testutil.RouteUnavailable(err) {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "not found") {
			t.Fatalf("history get rejected new fields: %v", err)
		}
		return
	}
	if err == nil && result.Transfers == nil {
		result.Transfers = []models.OrderTransfer{}
	}
}

func TestOrdersListOpen(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	limit := 10
	result := testutil.CallRequired(t, "orders.list_open", func() (models.OrdersList, error) {
		return client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
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
		return client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
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

func TestOrdersGetStateOnlyAndExecutionHistory(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "orders.list_history", func() (models.OrdersList, error) {
		return client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 10, false, false, nil)
	})
	if len(listed.Orders) == 0 {
		limit := 10
		listed = testutil.CallRequired(t, "orders.list_open", func() (models.OrdersList, error) {
			return client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
		})
	}
	if len(listed.Orders) == 0 {
		t.Skip("no orders on devnet; cannot exercise lineage get")
	}
	sample := listed.Orders[0]
	includeHistory := false
	stateOnly := testutil.CallRequired(t, "orders.get_state_only", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByID(sample.OrderID), nil, false, false, models.GetOrderOptions{
			IncludeExecutionHistory: &includeHistory,
		})
	})
	if stateOnly.Order == nil || stateOnly.Order.OrderID != sample.OrderID {
		t.Fatalf("state-only get=%+v", stateOnly.Order)
	}
	if len(stateOnly.Trades) != 0 || len(stateOnly.Transfers) != 0 || stateOnly.NextPageToken != "" {
		t.Fatalf("state-only history leaked: %+v", stateOnly)
	}
	includeHistory = true
	limit := uint32(5)
	withHistory := testutil.CallRequired(t, "orders.get_history", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByID(sample.OrderID), nil, false, false, models.GetOrderOptions{
			IncludeExecutionHistory: &includeHistory,
			Limit:                   &limit,
		})
	})
	if withHistory.Order == nil || withHistory.Order.OrderID != sample.OrderID {
		t.Fatalf("history get=%+v", withHistory.Order)
	}
	if withHistory.Order.Lineage != nil && (withHistory.Order.Lineage.ID == "" || withHistory.Order.Lineage.Generation < 1) {
		t.Fatalf("order lineage=%+v", withHistory.Order.Lineage)
	}
}

func TestOrdersListHistory(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "orders.list_history", func() (models.OrdersList, error) {
		return client.Orders.ListHistory(ctx, nil, nil, &symbol, nil, nil, 5, false, false, nil)
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
