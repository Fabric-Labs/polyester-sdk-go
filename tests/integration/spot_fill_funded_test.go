//go:build integration

package integration_test

import (
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestSpotFill(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)
	if !testutil.TradeE2EEnabled() {
		t.Skip("Set POLYESTER_TEST_TRADE_E2E=1 to run spot fill e2e")
	}

	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	maker, _, err := testutil.MakerClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if maker == nil {
		t.Skip("Set POLYESTER_TEST_MAKER_API_KEY_ID and POLYESTER_TEST_MAKER_API_PRIVATE_KEY for limit fill e2e (devnet orderbook often has no liquidity)")
	}
	defer maker.Close()
	_, _ = testutil.HydrateSpotRaw(ctx, maker)

	makerCID := testutil.UniqueClientOrderID("maker-fill")
	takerCID := testutil.UniqueClientOrderID("taker-fill")
	makerOrderCreated := false
	takerOrderCreated := false

	defer func() {
		if takerOrderCreated {
			_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
		}
		if makerOrderCreated {
			_, _ = maker.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
		}
	}()

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("trade symbol %s is not in spot config", symbol)
	}
	zipper := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})

	price := strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRADE_PRICE"))
	if price == "" {
		price = testutil.FarAboveBuyStopPrice(symbol, pair)
	}
	qty := strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRADE_QTY"))
	if qty == "" {
		qty = testutil.MinBaseQtyForPair(pair, price)
	}

	qtyDecimal, err := testutil.DecimalStringRequired(qty)
	if err != nil {
		t.Fatal(err)
	}
	priceDecimal, err := testutil.DecimalStringRequired(price)
	if err != nil {
		t.Fatal(err)
	}

	quoteAssetID := testutil.QuoteAssetIDForSymbol(spotRaw, symbol, zipper)
	baseAssetID := testutil.BaseAssetIDForSymbol(spotRaw, symbol, zipper)
	if quoteAssetID == nil || baseAssetID == nil {
		t.Fatal("expected quote and base asset ids")
	}

	takerBefore := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	takerQuoteBefore := testutil.TradingBalanceHuman(takerBefore, *quoteAssetID)
	takerBaseBefore := testutil.TradingBalanceHuman(takerBefore, *baseAssetID)
	requiredQuote := new(big.Rat).Mul(priceDecimal, qtyDecimal)
	if takerQuoteBefore.Cmp(requiredQuote) < 0 {
		t.Skipf("taker quote balance %s below required %s for %s %s at %s",
			takerQuoteBefore.FloatString(8), requiredQuote.FloatString(8), qty, symbol, price)
	}

	makerBefore := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return maker.Balances.List(ctx, nil, nil)
	})
	makerQuoteBefore := testutil.TradingBalanceHuman(makerBefore, *quoteAssetID)
	makerBaseBefore := testutil.TradingBalanceHuman(makerBefore, *baseAssetID)
	if makerBaseBefore.Cmp(qtyDecimal) < 0 {
		t.Skipf("maker base balance %s below fill quantity %s", makerBaseBefore.FloatString(8), qty)
	}

	tif := "gtc"
	makerCreated, err := maker.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "sell",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &makerCID,
		PostOnly:      true,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet order placement unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if makerCreated.ClientOrderID != makerCID || makerCreated.OrderID == "" {
		t.Fatalf("maker create=%+v", makerCreated)
	}
	makerOrderCreated = true

	takerCreated, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &takerCID,
		PostOnly:      false,
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
	if takerCreated.ClientOrderID != takerCID || takerCreated.OrderID == "" {
		t.Fatalf("taker create=%+v", takerCreated)
	}
	takerOrderCreated = true

	takerDetail, err := testutil.WaitForFilledOrder(ctx, client, takerCID, 0)
	if err != nil {
		t.Fatal(err)
	}
	matchIDs := map[string]struct{}{}
	for _, trade := range takerDetail.Trades {
		if trade.MatchID != "" {
			matchIDs[trade.MatchID] = struct{}{}
		}
	}
	if len(matchIDs) == 0 {
		t.Fatal("expected taker match ids")
	}

	makerDetail, err := testutil.WaitForFilledOrder(ctx, maker, makerCID, 0)
	if err != nil {
		t.Fatal(err)
	}
	makerMatchIDs := map[string]struct{}{}
	for _, trade := range makerDetail.Trades {
		if trade.MatchID != "" {
			makerMatchIDs[trade.MatchID] = struct{}{}
		}
	}

	var matchID string
	for id := range matchIDs {
		if _, ok := makerMatchIDs[id]; ok {
			matchID = id
			break
		}
	}
	if matchID == "" {
		t.Fatal("expected overlapping match id between maker and taker")
	}

	takerTrade, err := testutil.WaitForTradeMatch(ctx, client, symbol, matchID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if takerTrade.Side != "buy" {
		t.Fatalf("taker trade side=%q want buy", takerTrade.Side)
	}

	makerTrade, err := testutil.WaitForTradeMatch(ctx, maker, symbol, matchID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if makerTrade.Side != "sell" {
		t.Fatalf("maker trade side=%q want sell", makerTrade.Side)
	}

	takerAfter := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	if testutil.TradingBalanceHuman(takerAfter, *baseAssetID).Cmp(takerBaseBefore) <= 0 {
		t.Fatal("expected taker base balance to increase")
	}
	if testutil.TradingBalanceHuman(takerAfter, *quoteAssetID).Cmp(takerQuoteBefore) >= 0 {
		t.Fatal("expected taker quote balance to decrease")
	}

	makerAfter := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return maker.Balances.List(ctx, nil, nil)
	})
	if testutil.TradingBalanceHuman(makerAfter, *baseAssetID).Cmp(makerBaseBefore) >= 0 {
		t.Fatal("expected maker base balance to decrease")
	}
	if testutil.TradingBalanceHuman(makerAfter, *quoteAssetID).Cmp(makerQuoteBefore) <= 0 {
		t.Fatal("expected maker quote balance to increase")
	}
}
