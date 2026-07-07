//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

const btcUSDT = "BTC-USDT"

func TestMarketBuyMutation(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := btcUSDT
	if override := testutil.EnvTradeSymbol(); override != "" {
		symbol = override
	}
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("symbol %s is not in spot config", symbol)
	}
	price := testutil.ResolvePostOnlyBuyLimitPrice(ctx, client, symbol, pair)
	qty := testutil.MinBaseQtyForPair(pair, price)
	refPrice := testutil.MarketRefPrice(ctx, client, symbol, "buy", pair)

	clientOrderID := testutil.UniqueClientOrderID("mkt-buy")
	tif := "ioc"
	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:               &symbol,
		Side:                 "buy",
		OrderType:            "market",
		TIF:                  &tif,
		Qty:                  qty,
		ClientOrderID:        &clientOrderID,
		MarketClientRefPrice: &refPrice,
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
	if created.ClientOrderID != clientOrderID || created.OrderID == "" {
		t.Fatalf("create=%+v", created)
	}
	if created.Status == "canceled" || created.Status == "rejected" || created.Status == "filled" {
		return
	}

	defer func() {
		_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
	}()

	detail, err := testutil.WaitForTerminalOrder(ctx, client, clientOrderID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Order == nil {
		t.Fatal("expected order detail")
	}
	if detail.Order.OrderType != "" && detail.Order.OrderType != "market" {
		t.Fatalf("order_type=%q want market", detail.Order.OrderType)
	}
	switch detail.Order.Status {
	case "canceled", "rejected", "filled":
	default:
		t.Fatalf("unexpected terminal status %q", detail.Order.Status)
	}
}

func TestMarketSellMutation(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := btcUSDT
	if override := testutil.EnvTradeSymbol(); override != "" {
		symbol = override
	}

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("symbol %s is not in spot config", symbol)
	}
	price := testutil.ResolvePostOnlyBuyLimitPrice(ctx, client, symbol, pair)
	qty := testutil.MinBaseQtyForPair(pair, price)
	refPrice := testutil.MarketRefPrice(ctx, client, symbol, "sell", pair)
	testutil.RequireTradingBaseBalanceForSymbol(t, ctx, client, symbol, qty)

	clientOrderID := testutil.UniqueClientOrderID("mkt-sell")
	tif := "ioc"
	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:               &symbol,
		Side:                 "sell",
		OrderType:            "market",
		TIF:                  &tif,
		Qty:                  qty,
		ClientOrderID:        &clientOrderID,
		MarketClientRefPrice: &refPrice,
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
	if created.ClientOrderID != clientOrderID || created.OrderID == "" {
		t.Fatalf("create=%+v", created)
	}
	if created.Status == "canceled" || created.Status == "rejected" || created.Status == "filled" {
		return
	}

	defer func() {
		_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
	}()

	detail, err := testutil.WaitForTerminalOrder(ctx, client, clientOrderID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Order == nil {
		t.Fatal("expected order detail")
	}
	if detail.Order.OrderType != "" && detail.Order.OrderType != "market" {
		t.Fatalf("order_type=%q want market", detail.Order.OrderType)
	}
	switch detail.Order.Status {
	case "canceled", "rejected", "filled":
	default:
		t.Fatalf("unexpected terminal status %q", detail.Order.Status)
	}
}
