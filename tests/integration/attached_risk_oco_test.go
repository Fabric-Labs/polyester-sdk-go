//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// TestAttachedOCORoundTrip creates a resting buy with an attached take-profit
// and stop-loss linked as OCO, reads both legs back, replaces the policy with
// a modify, reads the replacement, and cancels the order by ID.
func TestAttachedOCORoundTrip(t *testing.T) {
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
	parentTicks, err := codecs.ParsePriceTicks(price, "price")
	if err != nil {
		t.Fatal(err)
	}
	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	tick := pairTickTicks(t, testutil.PairForSymbol(spotRaw, symbol))
	parent := int64(parentTicks)

	clientOrderID := testutil.UniqueClientOrderID("oco")
	tif := "gtc"
	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &clientOrderID,
		PostOnly:      true,
		AttachedRisk:  ocoInput(alignDown(parent*2, tick), alignDown(parent/2, tick)),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByID(created.OrderID), &symbol, nil, nil)
	}()
	if created.TakeProfitTriggerID == "" || created.StopLossTriggerID == "" || created.TrailingStopTriggerID != "" {
		t.Fatalf("create trigger IDs=%+v", created.AttachedRiskTriggerIDs)
	}
	requireOCO(t, ctx, client, created.OrderID, alignDown(parent*2, tick), alignDown(parent/2, tick),
		created.TakeProfitTriggerID, created.StopLossTriggerID)

	modified, err := client.Orders.ModifyWith(ctx, nil, services.ModifyParams{
		Symbol:          symbol,
		Key:             models.OrderKeyByID(created.OrderID),
		NewAttachedRisk: ocoInput(alignDown(parent*3, tick), alignDown(parent/3, tick)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if modified.ActionTaken == "" || modified.OldOrderID != created.OrderID || modified.FinalOrderID == "" {
		t.Fatalf("modify result=%+v", modified)
	}
	t.Logf("modify action=%s old=%s final=%s trigger IDs=%+v", modified.ActionTaken, modified.OldOrderID, modified.FinalOrderID, modified.AttachedRiskTriggerIDs)
	if modified.TakeProfitTriggerID == "" || modified.StopLossTriggerID == "" || modified.TrailingStopTriggerID != "" {
		t.Fatalf("modify trigger IDs=%+v", modified.AttachedRiskTriggerIDs)
	}
	requireOCO(t, ctx, client, modified.FinalOrderID, alignDown(parent*3, tick), alignDown(parent/3, tick),
		modified.TakeProfitTriggerID, modified.StopLossTriggerID)

	if modified.FinalOrderID != created.OrderID {
		defer func() {
			_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByID(modified.FinalOrderID), &symbol, nil, nil)
		}()
	}
	if _, err := client.Orders.Cancel(ctx, nil, models.OrderKeyByID(modified.FinalOrderID), &symbol, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WaitForNoOpenOrder(ctx, client, clientOrderID, 50, 0); err != nil {
		t.Fatal(err)
	}
}

func ocoInput(takeProfitTicks, stopLossTicks int64) *models.AttachedRiskInput {
	return &models.AttachedRiskInput{
		TakeProfit: &models.RiskLegInput{TriggerPrice: models.PriceFromTicksInt(takeProfitTicks)},
		StopLoss:   &models.RiskLegInput{TriggerPrice: models.PriceFromTicksInt(stopLossTicks)},
		OCO:        true,
	}
}

// requireOCO polls the order until it reports the expected attached legs. Leg
// trigger IDs, which the write result must carry, must match the read.
func requireOCO(t *testing.T, ctx context.Context, client *polyester.Client, orderID string, takeProfitTicks, stopLossTicks int64, takeProfitID, stopLossID string) {
	t.Helper()
	include := false
	var last *models.Order
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(orderID), nil, true, true, models.GetOrderOptions{
			IncludeExecutionHistory: &include,
		})
		if err == nil && got.Order != nil {
			last = got.Order
			risk := got.Order.AttachedRisk
			if risk != nil && risk.Oco && risk.TakeProfit != nil && risk.StopLoss != nil &&
				risk.TakeProfit.TriggerPrice.Ticks() == takeProfitTicks && risk.StopLoss.TriggerPrice.Ticks() == stopLossTicks {
				if risk.TrailingStop != nil {
					t.Fatalf("unexpected trailing stop: %+v", risk.TrailingStop)
				}
				requireLegTrigger(t, "take_profit", risk.TakeProfit.State, takeProfitID)
				requireLegTrigger(t, "stop_loss", risk.StopLoss.State, stopLossID)
				t.Logf("order %s attached OCO tp=%d sl=%d tp_state=%+v sl_state=%+v", orderID,
					takeProfitTicks, stopLossTicks, risk.TakeProfit.State, risk.StopLoss.State)
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if last == nil {
		t.Fatalf("order %s never readable", orderID)
	}
	t.Fatalf("order %s attached risk=%+v want OCO tp=%d sl=%d", orderID, last.AttachedRisk, takeProfitTicks, stopLossTicks)
}

func requireLegTrigger(t *testing.T, leg string, state *models.AttachedRiskLegState, writeID string) {
	t.Helper()
	if state == nil || state.TriggerID == "" {
		t.Fatalf("%s state missing trigger ID: %+v", leg, state)
	}
	if writeID == "" {
		t.Fatalf("%s: write result carried no trigger ID", leg)
	}
	if state.TriggerID != writeID {
		t.Fatalf("%s trigger ID read=%s write=%s", leg, state.TriggerID, writeID)
	}
}

func pairTickTicks(t *testing.T, pair map[string]any) int64 {
	t.Helper()
	tick := "0.01"
	for _, key := range []string{"tickSize", "tick_size"} {
		if raw, ok := pair[key].(string); ok && raw != "" {
			tick = raw
			break
		}
	}
	ticks, err := codecs.ParsePriceTicks(tick, "tick_size")
	if err != nil || ticks == 0 {
		t.Fatalf("tick size %q: %v", tick, err)
	}
	return int64(ticks)
}

func alignDown(value, tick int64) int64 {
	return value - value%tick
}
