//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// TestContractReservedBalance requires a resting order and observes both edges
// of its reservation. A no-op or an unexercised scenario cannot pass.
func TestContractReservedBalance(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()
	symbol := testutil.TradeSymbol(t, client, ctx)
	raw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(raw, symbol)
	if pair == nil {
		t.Fatal("missing market")
	}
	zipper, err := client.Zipper.GetDepositWithdrawConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	quote := testutil.QuoteAssetIDForSymbol(raw, symbol, zipper)
	if quote == nil {
		t.Fatal("missing quote asset")
	}
	before, err := client.Balances.List(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reservedBefore := testutil.ReservedBalance(before, *quote)
	book, err := client.Orderbook.Get(ctx, symbol, 5)
	if err != nil || len(book.Bids) == 0 {
		t.Fatalf("book bids unavailable: %v", err)
	}
	tick, err := codecs.ParsePriceTicks(fmt.Sprint(pair["tickSize"]), "tick_size")
	if err != nil || tick == 0 {
		tick, err = codecs.ParsePriceTicks(fmt.Sprint(pair["tick_size"]), "tick_size")
	}
	if err != nil || tick == 0 {
		t.Fatal("missing tick size")
	}
	priceTicks := book.Bids[0].Price.Ticks() / 2 / int64(tick) * int64(tick)
	if priceTicks <= 0 {
		t.Fatal("no far-from-market price")
	}
	price := new(big.Rat).SetFrac(big.NewInt(priceTicks), big.NewInt(1_000_000_000)).FloatString(9)
	qty := testutil.MinBaseQtyForPair(pair, price)
	cid := testutil.UniqueClientOrderID("contract-reserve")
	tif := "gtc"
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cancelOpenTestOrders(t, cleanupCtx, client, symbol, map[string]struct{}{cid: {}}, "reservation")
	}()
	_, err = client.Orders.Create(ctx, models.CreateOrderRequest{Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif, Qty: models.QtyFromDecimal(qty), Price: pricePtr(models.PriceFromDecimal(price)), PostOnly: true, ClientOrderID: &cid}, nil)
	if err != nil {
		t.Fatal(err)
	}
	order, err := testutil.WaitForOpenOrder(ctx, client, cid, 100, 15*time.Second)
	if err != nil || order.OrderID == "" {
		t.Fatalf("post-only order never rested: %v", err)
	}
	wait := func(increased bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			current, e := client.Balances.List(ctx, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			value := testutil.ReservedBalance(current, *quote)
			if increased && value.Cmp(reservedBefore) > 0 || !increased && value.Cmp(reservedBefore) == 0 {
				t.Logf("reserved asset=%d before=%s observed=%s", *quote, reservedBefore, value)
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("reserved balance did not reach expected state increased=%v", increased)
	}
	wait(true)
	if _, err = client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(cid), &symbol, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err = waitNoOpenCIDs(ctx, client, symbol, map[string]struct{}{cid: {}}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	wait(false)
}

func TestContractFeesAndUnknownTrigger(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()
	fees, err := client.Fees.GetSpotFeeRates(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fees.FeeRates) == 0 {
		t.Fatal("no fee rates returned")
	}
	for _, row := range fees.FeeRates {
		if row.ReserveFeeRatePercent == "" || row.PostOnlyReserveFeeRatePercent == "" || row.Breakdown == nil {
			t.Fatalf("new fee fields absent for symbol %d", row.SymbolID)
		}
	}
	t.Logf("fee reserve fields and breakdown present for %d markets", len(fees.FeeRates))
	id := codecs.FormatID(^uint64(0) - 101)
	_, getErr := client.Triggers.Get(ctx, nil, id, nil)
	_, eventsErr := client.Triggers.ListEvents(ctx, nil, id, nil, 5, nil, nil)
	if !testutil.IsNotFound(getErr) || !testutil.IsNotFound(eventsErr) {
		t.Fatalf("unknown trigger get=%v events=%v", getErr, eventsErr)
	}
	t.Log("GetTrigger and ListTriggerEvents both returned not_found")
}
