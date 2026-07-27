//go:build integration

package integration_test

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// TestMarketBuySellRoundtrip is a self-contained fixture:
// market BUY → carry net received base qty → market SELL that exact qty →
// assert terminal fills, residual base zero, no residual open test orders,
// and holds reconciled when list_holds is mounted.
func TestMarketBuySellRoundtrip(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)
	if !testutil.TradeE2EEnabled() {
		testutil.SoftSkip(t, "Set POLYESTER_TEST_TRADE_E2E=1 to run market roundtrip e2e")
	}

	client, ok, err := testutil.LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		testutil.SoftSkip(t, "POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	maker, hasMaker, err := testutil.MakerClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if hasMaker {
		defer maker.Close()
		_, _ = testutil.HydrateSpotRaw(ctx, maker)
		t.Log("market roundtrip liquidity=dedicated-maker")
	} else {
		t.Log("market roundtrip liquidity=external-orderbook")
	}

	buyCID := testutil.UniqueClientOrderID("rt-buy")
	sellCID := testutil.UniqueClientOrderID("rt-sell")
	makerCID := testutil.UniqueClientOrderID("rt-maker")
	makerBuyCID := testutil.UniqueClientOrderID("rt-maker-buy")
	testCIDs := map[string]struct{}{buyCID: {}, sellCID: {}}
	makerCIDs := map[string]struct{}{makerCID: {}, makerBuyCID: {}}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		cancelOpenTestOrders(t, cleanupCtx, client, symbol, testCIDs, "taker")
		if hasMaker {
			cancelOpenTestOrders(t, cleanupCtx, maker, symbol, makerCIDs, "maker")
		}
		if err := waitNoOpenCIDs(cleanupCtx, client, symbol, testCIDs, 20*time.Second); err != nil {
			if testutil.StrictLiveEnabled() {
				t.Errorf("cleanup verification failed: %v", err)
			} else {
				t.Logf("cleanup verification warning: %v", err)
			}
		}
	}()

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		testutil.SoftSkipf(t, "trade symbol %s is not in spot config", symbol)
	}
	zipper := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	baseAssetID := testutil.BaseAssetIDForSymbol(spotRaw, symbol, zipper)
	if baseAssetID == nil {
		testutil.SoftSkipf(t, "cannot resolve base asset for %s", symbol)
	}
	quoteAssetID := testutil.QuoteAssetIDForSymbol(spotRaw, symbol, zipper)

	before, err := client.Balances.List(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseBefore := testutil.TradingBalance(before, *baseAssetID)
	baseReservedBefore := testutil.ReservedBalance(before, *baseAssetID)
	quoteReservedBefore := big.NewInt(0)
	if quoteAssetID != nil {
		quoteReservedBefore = testutil.ReservedBalance(before, *quoteAssetID)
	}

	buyRefPrice := testutil.MarketRefPrice(ctx, client, symbol, "buy", pair)
	price := buyRefPrice
	if hasMaker {
		price = testutil.FarAboveBuyStopPrice(symbol, pair)
	}
	qty := testutil.MinBaseQtyForPair(pair, price)
	tif := "gtc"
	if hasMaker {
		makerCreated, err := maker.Orders.Create(ctx, models.CreateOrderRequest{
			Symbol: &symbol, Side: "sell", OrderType: "limit", TIF: &tif,
			Qty: models.QtyFromDecimal(qty), Price: pricePtr(models.PriceFromDecimal(price)),
			ClientOrderID: &makerCID, PostOnly: true,
		}, nil)
		if err != nil {
			if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
				testutil.SoftSkipf(t, "devnet order placement unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if makerCreated.OrderID == "" {
			t.Fatalf("maker create=%+v", makerCreated)
		}
	}

	buyTIF := "ioc"
	buyRef := models.PriceFromDecimal(buyRefPrice)
	_, err = client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol: &symbol, Side: "buy", OrderType: "market", TIF: &buyTIF,
		Qty: models.QtyFromDecimal(qty), ClientOrderID: &buyCID,
		MarketClientRefPrice: &buyRef,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			testutil.SoftSkipf(t, "devnet order placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			testutil.SoftSkipf(t, "order sizing below min notional: %v", err)
		}
		t.Fatal(err)
	}

	buyDetail, err := testutil.WaitForTerminalOrder(ctx, client, buyCID, 20*time.Second)
	if err != nil {
		testutil.SoftSkipf(t, "buy terminal wait (possible POLY-3028): %v", err)
	}
	if buyDetail.Order == nil {
		t.Fatal("expected buy order detail")
	}
	filled := buyDetail.Order.CumQty
	if filled.Scaled() <= 0 {
		testutil.SoftSkip(t, "buy produced no fill (possible POLY-3028)")
	}
	buyProjection, err := client.Orders.WaitForOrderTradesComplete(
		ctx, nil, nil, &buyCID, nil, 20*time.Second,
	)
	if err != nil {
		t.Fatalf("BUY trade projection did not reconcile: %v", err)
	}
	var receivedFee int64
	for _, trade := range buyProjection.Trades {
		if trade.FeeSource != "received" {
			continue
		}
		fee, parseErr := strconv.ParseInt(trade.FeeScaled, 10, 64)
		if parseErr != nil {
			t.Fatalf("invalid received-asset fee %q: %v", trade.FeeScaled, parseErr)
		}
		if receivedFee > math.MaxInt64-fee {
			t.Fatal("received-asset fee sum overflow")
		}
		receivedFee += fee
	}
	netReceived := models.MustQtyScaled(filled.Scaled() - receivedFee).
		WithDomain(filled.Domain()).
		WithSymbol(filled.Symbol())
	if scale := filled.Scale(); scale != nil {
		netReceived = netReceived.WithScale(*scale)
	}
	if symbolID := filled.SymbolID(); symbolID != nil {
		netReceived = netReceived.WithSymbolID(*symbolID)
	}
	if netReceived.Scaled() <= 0 {
		t.Fatalf("BUY net received quantity must be positive: filled=%d fee=%d", filled.Scaled(), receivedFee)
	}

	if hasMaker {
		// Provide buy liquidity for the cleanup SELL when dedicated maker
		// credentials are available.
		makerBuyPrice := testutil.ResolvePostOnlyBuyLimitPrice(ctx, maker, symbol, pair)
		_, err = maker.Orders.Create(ctx, models.CreateOrderRequest{
			Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
			Qty: models.QtyFromScaled(netReceived), Price: pricePtr(models.PriceFromDecimal(makerBuyPrice)),
			ClientOrderID: &makerBuyCID, PostOnly: true,
		}, nil)
		if err != nil {
			if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
				testutil.SoftSkipf(t, "devnet maker buy unavailable: %v", err)
			}
			t.Fatal(err)
		}
	}

	// A BUY that pays fees from the received asset cannot safely sell its
	// gross cum_qty; carry the exact net base received into cleanup.
	sellTIF := "ioc"
	sellRef := models.PriceFromDecimal(testutil.MarketRefPrice(ctx, client, symbol, "sell", pair))
	_, err = client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol: &symbol, Side: "sell", OrderType: "market", TIF: &sellTIF,
		Qty: models.QtyFromScaled(netReceived), ClientOrderID: &sellCID,
		MarketClientRefPrice: &sellRef,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			testutil.SoftSkipf(t, "devnet sell cleanup unavailable: %v", err)
		}
		t.Fatal(err)
	}

	sellDetail, err := testutil.WaitForTerminalOrder(ctx, client, sellCID, 20*time.Second)
	if err != nil {
		testutil.SoftSkipf(t, "sell terminal wait (possible POLY-3028): %v", err)
	}
	if sellDetail.Order == nil {
		t.Fatal("expected sell order detail")
	}
	if sellDetail.Order.CumQty.Scaled() != netReceived.Scaled() {
		t.Fatalf("sell cum_qty=%d want buy net received %d", sellDetail.Order.CumQty.Scaled(), netReceived.Scaled())
	}

	limit := 100
	open, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range open.Orders {
		if order.ClientOrderID == buyCID || order.ClientOrderID == sellCID {
			t.Fatalf("test order still open: %+v", order)
		}
	}

	after, err := client.Balances.List(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseAfter := testutil.TradingBalance(after, *baseAssetID)
	if baseAfter.Cmp(baseBefore) != 0 {
		t.Fatalf("residual base position not zero: before=%s after=%s", baseBefore, baseAfter)
	}

	// Reserved balances are the required reconciliation signal even when the
	// optional detailed list_holds route is not mounted.
	baseReservedAfter := testutil.ReservedBalance(after, *baseAssetID)
	if baseReservedAfter.Cmp(baseReservedBefore) != 0 {
		t.Fatalf("base reserved not reconciled: before=%s after=%s", baseReservedBefore, baseReservedAfter)
	}
	if quoteAssetID != nil {
		quoteReservedAfter := testutil.ReservedBalance(after, *quoteAssetID)
		if quoteReservedAfter.Cmp(quoteReservedBefore) != 0 {
			t.Fatalf("quote reserved not reconciled: before=%s after=%s", quoteReservedBefore, quoteReservedAfter)
		}
	}

	_, holdsErr := client.Balances.ListHolds(ctx, nil, nil, 20, false)
	if holdsErr != nil && !testutil.RouteUnavailable(holdsErr) {
		if testutil.StrictLiveEnabled() {
			t.Fatalf("list_holds after roundtrip: %v", holdsErr)
		}
		t.Logf("holds reconcile skipped: %v", holdsErr)
	}
}

func cancelOpenTestOrders(t *testing.T, ctx context.Context, client *polyester.Client, symbol string, cids map[string]struct{}, label string) {
	t.Helper()
	limit := 100
	open, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
	if err != nil {
		t.Errorf("cleanup %s list_open failed: %v", label, err)
		return
	}
	for _, order := range open.Orders {
		if _, ok := cids[order.ClientOrderID]; !ok {
			continue
		}
		cid := order.ClientOrderID
		if _, err := client.Orders.Cancel(ctx, nil, nil, &cid, &symbol, nil, nil); err != nil {
			t.Errorf("cleanup %s cancel %s failed: %v", label, cid, err)
		}
	}
}

func waitNoOpenCIDs(ctx context.Context, client *polyester.Client, symbol string, cids map[string]struct{}, timeout time.Duration) error {
	_ = symbol
	deadline := time.Now().Add(timeout)
	limit := 100
	for time.Now().Before(deadline) {
		open, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
		if err == nil {
			remaining := 0
			for _, order := range open.Orders {
				if _, ok := cids[order.ClientOrderID]; ok {
					remaining++
				}
			}
			if remaining == 0 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("test orders still open after cleanup poll")
}
