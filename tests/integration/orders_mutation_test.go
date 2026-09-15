//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderRoundTripMutation(t *testing.T) {
	testutil.RequireAccountWideCleanup(t)
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	price, qty, err := testutil.USDTFundedBuyLimitParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	clientOrderID := testutil.UniqueClientOrderID("e2e")
	postOnly := true
	tif := "gtc"

	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &clientOrderID,
		PostOnly:      postOnly,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet order placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("order sizing below devnet minimum notional: %v", err)
		}
		t.Fatal(err)
	}
	if created.ClientOrderID != clientOrderID {
		t.Fatalf("client_order_id=%q want %q", created.ClientOrderID, clientOrderID)
	}
	if created.OrderID == "" {
		t.Fatal("expected order_id from create")
	}

	openOrder, err := testutil.WaitForOpenOrder(ctx, client, clientOrderID, 50, 0)
	if err != nil {
		var notIndexed *testutil.DevnetOrderNotIndexedError
		if errors.As(err, &notIndexed) {
			t.Skip("devnet order create accepted but orders read APIs never indexed the order; check OMS read path on devnet")
		}
		t.Fatal(err)
	}
	if openOrder.ClientOrderID != clientOrderID {
		t.Fatalf("open order client_order_id=%q", openOrder.ClientOrderID)
	}

	detail := testutil.CallRequired(t, "orders.get", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false)
	})
	if detail.Order == nil || detail.Order.ClientOrderID != clientOrderID {
		t.Fatalf("detail=%+v", detail.Order)
	}
	if detail.Order.OrigQty.Scaled() <= 0 {
		t.Fatalf("expected current accepted orig_qty, got %+v", detail.Order.OrigQty)
	}

	defer func() {
		_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
	}()

	cancelled, err := client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(clientOrderID), &symbol, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status == "" {
		t.Fatalf("cancel status empty: %+v", cancelled)
	}
	if err := testutil.WaitForNoOpenOrder(ctx, client, clientOrderID, 50, 0); err != nil {
		t.Fatal(err)
	}
}

func TestOrderModifyAdvancesLineage(t *testing.T) {
	testutil.RequireMutation(t)
	testutil.RequireFunded(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	price, qty, err := testutil.USDTFundedBuyLimitParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	originalCID := testutil.UniqueClientOrderID("lin")
	replacementCID := testutil.UniqueClientOrderID("linr")
	tif := "gtc"
	postOnly := true
	behavior := "replace_only"

	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &originalCID,
		PostOnly:      postOnly,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet order placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("order sizing below devnet minimum notional: %v", err)
		}
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(replacementCID), &symbol, nil, nil)
		_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(originalCID), &symbol, nil, nil)
		if created.OrderID != "" {
			_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByID(created.OrderID), &symbol, nil, nil)
		}
	}()

	if _, err := waitListedOpen(t, ctx, client, originalCID); err != nil {
		t.Skip(err.Error())
	}
	include := false
	first, err := client.Orders.Get(ctx, nil, models.OrderKeyByClientID(originalCID), nil, false, false, models.GetOrderOptions{
		IncludeExecutionHistory: &include,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Order == nil {
		t.Fatal("expected original order")
	}
	originalLineageID := created.OrderID
	originalGeneration := uint32(1)
	if first.Order.Lineage != nil && first.Order.Lineage.ID != "" {
		originalLineageID = first.Order.Lineage.ID
		originalGeneration = first.Order.Lineage.Generation
	}
	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	newPrice := oneTickBelow(t, testutil.PairForSymbol(spotRaw, symbol), first.Order.Price.Ticks(), price)

	modified, err := client.Orders.Modify(
		ctx, nil, symbol, models.OrderKeyByClientID(originalCID), nil, nil,
		pricePtr(models.PriceFromDecimal(newPrice)), nil, &behavior, &replacementCID,
	)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet modify unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if modified.ActionTaken == "" || modified.OldOrderID != created.OrderID || modified.FinalOrderID == "" {
		t.Fatalf("modify result=%+v created=%s", modified, created.OrderID)
	}

	if _, err := waitListedOpen(t, ctx, client, replacementCID); err != nil {
		t.Skip(err.Error())
	}
	history, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(modified.FinalOrderID), nil, false, false, models.GetOrderOptions{
		IncludeExecutionHistory: &include,
	})
	if err != nil {
		t.Fatal(err)
	}
	if history.Order == nil || history.Order.Lineage == nil {
		t.Fatalf("replacement missing lineage: %+v", history.Order)
	}
	if history.Order.Lineage.ID != originalLineageID {
		t.Fatalf("lineage id=%q want %q", history.Order.Lineage.ID, originalLineageID)
	}
	if modified.FinalOrderID != created.OrderID && history.Order.Lineage.Generation != originalGeneration+1 {
		t.Fatalf("generation=%d want %d", history.Order.Lineage.Generation, originalGeneration+1)
	}
}

func waitListedOpen(t *testing.T, ctx context.Context, client *polyester.Client, clientOrderID string) (models.Order, error) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	limit := 50
	for time.Now().Before(deadline) {
		listed, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
		if err == nil {
			for _, order := range listed.Orders {
				if order.ClientOrderID == clientOrderID {
					return order, nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return models.Order{}, fmt.Errorf("open order %s was not visible in list_open", clientOrderID)
}

func oneTickBelow(t *testing.T, pair map[string]any, currentTicks int64, fallback string) string {
	t.Helper()
	tick := "0.01"
	if pair != nil {
		if raw, ok := pair["tickSize"].(string); ok && raw != "" {
			tick = raw
		} else if raw, ok := pair["tick_size"].(string); ok && raw != "" {
			tick = raw
		}
	}
	step, err := codecs.ParsePriceTicks(tick, "tick_size")
	if err != nil || step == 0 {
		t.Fatalf("tick size %q: %v", tick, err)
	}
	if currentTicks <= 0 {
		parsed, err := codecs.ParsePriceTicks(fallback, "price")
		if err != nil {
			t.Fatal(err)
		}
		currentTicks = int64(parsed)
	}
	next := currentTicks - int64(step)
	if next < int64(step) {
		next = int64(step)
	}
	return codecs.FormatPriceTicks(next)
}
