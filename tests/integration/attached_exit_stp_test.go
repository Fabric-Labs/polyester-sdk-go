//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// TestAttachedExitSelfTradePreventionMode rests a post-only buy at half the
// best bid with an attached take-profit and stop-loss and self-trade
// prevention expire_taker. The order and both legs read the mode back; the
// cancel ends the order and its legs.
func TestAttachedExitSelfTradePreventionMode(t *testing.T) {
	testutil.RequireMutation(t)
	testutil.RequireFunded(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)
	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	tick := pairTickTicks(t, pair)
	nearTicks, err := codecs.ParsePriceTicks(testutil.ResolvePostOnlyBuyLimitPrice(ctx, client, symbol, pair), "price")
	if err != nil {
		t.Fatal(err)
	}
	parent := alignDown(int64(nearTicks)/2, tick)
	price := codecs.FormatPriceTicks(parent)
	takeProfit, stopLoss := alignDown(parent*3, tick), alignDown(parent/2, tick)
	// Each leg's exit must meet the pair's minimums too, a market exit at its
	// maximum slippage price: size at 90% of the stop-loss, the lowest exit.
	qty := testutil.MinBaseQtyForPair(pair, codecs.FormatPriceTicks(alignDown(stopLoss*9/10, tick)))

	clientOrderID := testutil.UniqueClientOrderID("stp")
	tif := "gtc"
	mode := "expire_taker"
	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:                  &symbol,
		Side:                    "buy",
		OrderType:               "limit",
		TIF:                     &tif,
		Qty:                     models.QtyFromDecimal(qty),
		Price:                   pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID:           &clientOrderID,
		PostOnly:                true,
		SelfTradePreventionMode: &mode,
		AttachedRisk:            ocoInput(takeProfit, stopLoss),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByID(created.OrderID), &symbol, nil, nil)
	}()
	t.Logf("created order %s %s qty=%s price=%s (near bid %s) tp=%d sl=%d tp_trigger=%s sl_trigger=%s",
		created.OrderID, symbol, qty, price, codecs.FormatPriceTicks(int64(nearTicks)), takeProfit, stopLoss,
		created.TakeProfitTriggerID, created.StopLossTriggerID)
	requireOCO(t, ctx, client, created.OrderID, takeProfit, stopLoss, created.TakeProfitTriggerID, created.StopLossTriggerID)

	got, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(created.OrderID), nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Order == nil || got.Order.SelfTradePreventionMode != mode || !got.Order.PostOnly {
		t.Fatalf("order=%+v", got.Order)
	}
	t.Logf("order %s status=%s self_trade_prevention_mode=%s", got.Order.OrderID, got.Order.Status, got.Order.SelfTradePreventionMode)
	for leg, id := range map[string]string{"take_profit": created.TakeProfitTriggerID, "stop_loss": created.StopLossTriggerID} {
		trigger, err := client.Triggers.Get(ctx, nil, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s trigger %s status=%s self_trade_prevention_mode=%q parent=%s", leg, trigger.TriggerID, trigger.Status, trigger.SelfTradePreventionMode, trigger.ParentOrderID)
		if trigger.SelfTradePreventionMode != mode {
			t.Fatalf("%s trigger mode=%q want %q", leg, trigger.SelfTradePreventionMode, mode)
		}
	}

	if _, err := client.Orders.Cancel(ctx, nil, models.OrderKeyByID(created.OrderID), &symbol, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WaitForNoOpenOrder(ctx, client, clientOrderID, 50, 0); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for leg, id := range map[string]string{"take_profit": created.TakeProfitTriggerID, "stop_loss": created.StopLossTriggerID} {
		var status string
		for time.Now().Before(deadline) {
			trigger, err := client.Triggers.Get(ctx, nil, id, nil)
			if err == nil {
				status = trigger.Status
				if status == "cancelled" {
					break
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Logf("%s trigger %s after cancel status=%s", leg, id, status)
		if status != "cancelled" {
			t.Fatalf("%s trigger %s status=%s after the order's cancel", leg, id, status)
		}
	}
	final, err := client.Orders.Get(ctx, nil, models.OrderKeyByID(created.OrderID), nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("order %s final status=%s cancel_reason=%s", final.Order.OrderID, final.Order.Status, final.Order.CancelReason)
}
