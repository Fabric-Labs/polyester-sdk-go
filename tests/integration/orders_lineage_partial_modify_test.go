//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestPartialFillModifyKeepsPredecessorExecutionHistory(t *testing.T) {
	testutil.RequireMutation(t)
	testutil.RequireFunded(t)
	if !testutil.TradeE2EEnabled() {
		t.Skip("Set POLYESTER_TEST_TRADE_E2E=1 to spend against the live book")
	}

	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("%s is not in spot config", symbol)
	}

	book, err := client.Orderbook.Get(ctx, symbol, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Asks) == 0 || book.Asks[0].Price.Ticks() <= 0 {
		t.Skipf("no visible asks on %s", symbol)
	}

	askPrice := book.Asks[0].Price.Format()
	createPrice := askPrice
	originalCID := testutil.UniqueClientOrderID("pfill")
	replacementCID := testutil.UniqueClientOrderID("pfillr")
	makerCID := testutil.UniqueClientOrderID("pfillm")
	tif := "gtc"
	behavior := "replace_only"
	postOnly := true

	var maker *polyester.Client
	if second, ok, err := secondClientFromEnv(); err != nil {
		t.Fatal(err)
	} else if ok && len(book.Bids) > 0 && book.Bids[0].Price.Ticks() > 0 {
		maker = second
		defer maker.Close()
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
		makerTicks := book.Bids[0].Price.Ticks() + int64(step)
		if makerTicks >= book.Asks[0].Price.Ticks() {
			t.Skip("spread is too tight for an inside-spread maker")
		}
		createPrice = codecs.FormatPriceTicks(makerTicks)
		minQty := testutil.MinBaseQtyForPair(pair, createPrice)
		if _, err := testutil.HydrateSpotRaw(ctx, maker); err != nil {
			t.Fatal(err)
		}
		_, err = maker.Orders.Create(ctx, models.CreateOrderRequest{
			Symbol:        &symbol,
			Side:          "sell",
			OrderType:     "limit",
			TIF:           &tif,
			Qty:           models.QtyFromDecimal(minQty),
			Price:         pricePtr(models.PriceFromDecimal(createPrice)),
			ClientOrderID: &makerCID,
			PostOnly:      postOnly,
		}, nil)
		if err != nil {
			if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) || isSelfTrade(err) {
				t.Skipf("maker unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := waitListedOpen(t, ctx, maker, makerCID); err != nil {
			t.Skip(err.Error())
		}
	} else {
		t.Skip("no second key for a cheap partial fill; sweeping the ask is too expensive")
	}

	minQty := testutil.MinBaseQtyForPair(pair, createPrice)
	qty := doubleQtyString(minQty)

	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(createPrice)),
		ClientOrderID: &originalCID,
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
		if maker != nil {
			_, _ = maker.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(makerCID), &symbol, nil, nil)
		}
	}()

	partial, err := waitPartialFill(ctx, client, originalCID)
	if err != nil {
		t.Fatal(err)
	}
	if partial == nil {
		t.Skip("buy at best ask fully filled; book too deep to rest a remainder for ModifyOrder")
	}
	firstFillQty := partial.Order.CumQty.Scaled()
	if firstFillQty <= 0 {
		t.Fatal("expected predecessor fill quantity")
	}
	firstLineageID := created.OrderID
	firstGeneration := uint32(1)
	if partial.Order.Lineage != nil && partial.Order.Lineage.ID != "" {
		firstLineageID = partial.Order.Lineage.ID
		firstGeneration = partial.Order.Lineage.Generation
	}

	firstMatchIDs := map[string]bool{}
	if snapshot, err := getOrderHistory(ctx, client, models.OrderKeyByID(created.OrderID), 5, nil); err == nil {
		firstMatchIDs = matchIDs(snapshot.Trades)
	}

	restPrice := oneTickBelow(t, pair, partial.Order.Price.Ticks(), askPrice)
	if len(book.Bids) > 0 && book.Bids[0].Price.Ticks() > 0 {
		bidRest := oneTickBelow(t, pair, book.Bids[0].Price.Ticks(), askPrice)
		if bidTicks, perr := codecs.ParsePriceTicks(bidRest, "price"); perr == nil {
			if restTicks, rerr := codecs.ParsePriceTicks(restPrice, "price"); rerr == nil && bidTicks < restTicks {
				restPrice = bidRest
			}
		}
	}

	modified, err := client.Orders.Modify(
		ctx, nil, symbol, models.OrderKeyByID(created.OrderID), nil, nil,
		pricePtr(models.PriceFromDecimal(restPrice)), nil, &behavior, &replacementCID,
	)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) || testutil.IsNotFound(err) || strings.Contains(msg, "not found") {
			t.Skipf("remainder was not modifyable: %v", err)
		}
		t.Fatal(err)
	}
	if modified.OldOrderID != created.OrderID || modified.FinalOrderID == "" {
		t.Fatalf("modify result=%+v created=%s", modified, created.OrderID)
	}
	if _, err := waitListedOpen(t, ctx, client, replacementCID); err != nil {
		t.Skip(err.Error())
	}

	history, err := getOrderHistory(ctx, client, models.OrderKeyByID(modified.FinalOrderID), 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if history.Order == nil || history.Order.Lineage == nil {
		t.Fatalf("replacement missing lineage: %+v", history.Order)
	}
	if history.Order.Lineage.ID != firstLineageID {
		t.Fatalf("lineage id=%q want %q", history.Order.Lineage.ID, firstLineageID)
	}
	if history.Order.Lineage.Generation != firstGeneration+1 {
		t.Fatalf("generation=%d want %d", history.Order.Lineage.Generation, firstGeneration+1)
	}
	if history.Order.CumQty.Scaled() < firstFillQty {
		t.Fatalf("cum_qty=%d want >= %d", history.Order.CumQty.Scaled(), firstFillQty)
	}
	if len(history.Trades) == 0 {
		t.Fatal("POLY-4708: GetOrder history is empty after replacing a partially filled order")
	}
	afterMatchIDs := matchIDs(history.Trades)
	for id := range firstMatchIDs {
		if !afterMatchIDs[id] {
			t.Fatalf("predecessor fills missing after replace: before=%v after=%v", firstMatchIDs, afterMatchIDs)
		}
	}
	for _, trade := range history.Trades {
		if trade.Lineage != nil {
			if trade.Lineage.ID != firstLineageID {
				t.Fatalf("trade lineage id=%q want %q", trade.Lineage.ID, firstLineageID)
			}
			if trade.Lineage.Generation > history.Order.Lineage.Generation {
				t.Fatalf("trade generation=%d above order generation=%d", trade.Lineage.Generation, history.Order.Lineage.Generation)
			}
		}
	}

	page, err := getOrderHistory(ctx, client, models.OrderKeyByID(modified.FinalOrderID), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextPageToken != "" {
		pageTwo, err := getOrderHistory(ctx, client, models.OrderKeyByID(modified.FinalOrderID), 1, &page.NextPageToken)
		if err != nil {
			t.Fatal(err)
		}
		if pageTwo.Order == nil || pageTwo.Order.OrderID != modified.FinalOrderID {
			t.Fatalf("paged get lost order identity: %+v", pageTwo.Order)
		}
	}

	var scoped models.UserTradesList
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		scoped, lastErr = client.Trades.List(ctx, nil, nil, nil, nil, 5, nil, nil, models.ListUserTradesOptions{
			LineageID:         &firstLineageID,
			ThroughGeneration: &history.Order.Lineage.Generation,
			IncludeTransfers:  true,
		})
		if lastErr == nil {
			break
		}
		if !isQueryTimeout(lastErr) {
			t.Fatal(lastErr)
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		t.Fatalf("GetUserTrades lineage_id+transfers timed out after a real replacement: %v", lastErr)
	}
	scopedMatches := matchIDs(scoped.Trades)
	for id := range firstMatchIDs {
		if !scopedMatches[id] {
			t.Fatalf("lineage trades missing predecessor: before=%v after=%v", firstMatchIDs, scopedMatches)
		}
	}
	seenTX := map[string]struct{}{}
	for _, transfer := range scoped.Transfers {
		if transfer.TxID == "" {
			continue
		}
		if _, ok := seenTX[transfer.TxID]; ok {
			t.Fatalf("duplicate transfer tx_id=%s", transfer.TxID)
		}
		seenTX[transfer.TxID] = struct{}{}
	}
}

func secondClientFromEnv() (*polyester.Client, bool, error) {
	if client, ok, err := testutil.MakerClientFromEnv(); err != nil || ok {
		return client, ok, err
	}
	keyID := strings.TrimSpace(os.Getenv("POLYESTER_SUBACCOUNT_API_KEY_ID"))
	privateKey := strings.TrimSpace(os.Getenv("POLYESTER_SUBACCOUNT_API_PRIVATE_KEY"))
	primary := strings.TrimSpace(os.Getenv("POLYESTER_API_KEY_ID"))
	if keyID == "" || privateKey == "" || keyID == primary {
		return nil, false, nil
	}
	cfg := polyester.Config{APIKeyID: keyID, APIPrivateKey: privateKey, HydrateCatalogs: true}
	if apiURL := strings.TrimSpace(os.Getenv("POLYESTER_API_URL")); apiURL != "" {
		cfg.APIURL = apiURL
	}
	client, err := polyester.New(cfg)
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}

func doubleQtyString(minQty string) string {
	rat, err := testutil.DecimalStringRequired(minQty)
	if err != nil {
		return minQty
	}
	return new(big.Rat).Mul(rat, big.NewRat(2, 1)).FloatString(16)
}

func isSelfTrade(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "self-trade") || strings.Contains(msg, "self_trade")
}

func matchIDs(trades []models.UserTrade) map[string]bool {
	out := map[string]bool{}
	for _, trade := range trades {
		if trade.MatchID != "" {
			out[trade.MatchID] = true
		}
	}
	return out
}

func isQueryTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "query aborted")
}

func getOrderHistory(ctx context.Context, client *polyester.Client, key models.OrderKey, limit uint32, pageToken *string) (models.GetOrderResult, error) {
	include := true
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		result, err := client.Orders.Get(ctx, nil, key, nil, false, false, models.GetOrderOptions{
			IncludeExecutionHistory: &include,
			Limit:                   &limit,
			PageToken:               pageToken,
		})
		if err == nil {
			return result, nil
		}
		last = err
		if !isQueryTimeout(err) {
			return models.GetOrderResult{}, err
		}
		time.Sleep(time.Second)
	}
	return models.GetOrderResult{}, fmt.Errorf("GetOrder execution history timed out: %w", last)
}

func waitPartialFill(ctx context.Context, client *polyester.Client, clientOrderID string) (*models.GetOrderResult, error) {
	include := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false, models.GetOrderOptions{
			IncludeExecutionHistory: &include,
		})
		if err == nil && detail.Order != nil {
			cum := detail.Order.CumQty.Scaled()
			leaves := detail.Order.LeavesQty.Scaled()
			if cum > 0 && leaves > 0 {
				return &detail, nil
			}
			if detail.Order.Status == "filled" || (cum > 0 && leaves == 0) {
				return nil, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("order %s never partially filled", clientOrderID)
}
