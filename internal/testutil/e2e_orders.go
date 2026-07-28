package testutil

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// UniqueClientOrderID returns a unique client order id for tests.
func UniqueClientOrderID(prefix string) string {
	if prefix == "" {
		prefix = "test"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// USDTFundedBuyLimitParams returns price and qty for a post-only USDT-quoted buy.
func USDTFundedBuyLimitParams(ctx context.Context, client *polyester.Client, symbol string) (price, qty string, err error) {
	spotRaw, err := HydrateSpotRaw(ctx, client)
	if err != nil {
		return "", "", err
	}
	pair := PairForSymbol(spotRaw, symbol)
	price = ResolvePostOnlyBuyLimitPrice(ctx, client, symbol, pair)
	qty = MinBaseQtyForPair(pair, price)
	return price, qty, nil
}

// RequireTradingBalanceForSymbol skips when quote trading balance is below the configured minimum.
func RequireTradingBalanceForSymbol(t *testing.T, ctx context.Context, client *polyester.Client, symbol string) {
	t.Helper()
	if SkipFundingCheck() {
		return
	}
	spotRaw := spotRawFromClient(t, client, ctx)
	zipper := CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	quoteAssetID := QuoteAssetIDForSymbol(spotRaw, symbol, zipper)
	if quoteAssetID == nil {
		t.Skipf("cannot resolve quote asset for %s", symbol)
	}
	balances := CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	balance := TradingBalanceHuman(balances, *quoteAssetID)
	minimum, ok := new(big.Rat).SetString(MinTradingQuoteRequired().String())
	if !ok {
		minimum = big.NewRat(10, 1)
	}
	if balance.Cmp(minimum) < 0 {
		t.Skipf("trading balance %s below minimum %s for asset %d; fund devnet or set POLYESTER_TEST_SKIP_FUNDING_CHECK=1",
			balance.FloatString(8), minimum.FloatString(8), *quoteAssetID)
	}
}

var openOrderStatuses = map[string]struct{}{
	"pending": {}, "working": {}, "pending_cancel": {},
}

var terminalOrderStatuses = map[string]struct{}{
	"canceled": {}, "rejected": {}, "filled": {},
}

// WaitForOpenOrder polls until an order is visible as open.
func WaitForOpenOrder(ctx context.Context, client *polyester.Client, clientOrderID string, limit int, timeout time.Duration) (models.Order, error) {
	if limit <= 0 {
		limit = 50
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastStatus string
	for time.Now().Before(deadline) {
		detail, err := client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false)
		if err == nil && detail.Order != nil && detail.Order.ClientOrderID == clientOrderID {
			order := *detail.Order
			if order.Status == "" {
				return order, nil
			}
			if _, ok := openOrderStatuses[order.Status]; ok {
				return order, nil
			}
			lastStatus = order.Status
			if _, terminal := terminalOrderStatuses[order.Status]; terminal {
				return models.Order{}, fmt.Errorf("order %s reached terminal status %q", clientOrderID, order.Status)
			}
		}
		listed, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
		if err == nil {
			for _, order := range listed.Orders {
				if order.ClientOrderID == clientOrderID {
					return order, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return models.Order{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	msg := fmt.Sprintf("open order %s was not visible within %s", clientOrderID, timeout)
	if lastStatus != "" {
		return models.Order{}, &DevnetOrderNotIndexedError{Msg: msg + " (last get status: " + lastStatus + ")"}
	}
	return models.Order{}, &DevnetOrderNotIndexedError{Msg: msg}
}

// WaitForNoOpenOrder polls until an order is no longer open.
func WaitForNoOpenOrder(ctx context.Context, client *polyester.Client, clientOrderID string, limit int, timeout time.Duration) error {
	if limit <= 0 {
		limit = 50
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
		if err != nil {
			return err
		}
		found := false
		for _, order := range listed.Orders {
			if order.ClientOrderID == clientOrderID {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("order %s still open after %s", clientOrderID, timeout)
}

// WaitForTerminalOrder polls until an order reaches a terminal status.
func WaitForTerminalOrder(ctx context.Context, client *polyester.Client, clientOrderID string, timeout time.Duration) (models.GetOrderResult, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastDetail models.GetOrderResult
	for time.Now().Before(deadline) {
		detail, err := client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false)
		if err == nil && detail.Order != nil && detail.Order.ClientOrderID == clientOrderID {
			lastDetail = detail
			if _, ok := terminalOrderStatuses[detail.Order.Status]; ok {
				return detail, nil
			}
		} else if err != nil && !IsNotFound(err) {
			return models.GetOrderResult{}, err
		}
		select {
		case <-ctx.Done():
			return models.GetOrderResult{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastDetail.Order != nil && lastDetail.Order.Status != "" {
		return models.GetOrderResult{}, fmt.Errorf("order %s stuck in status %q after %s", clientOrderID, lastDetail.Order.Status, timeout)
	}
	return models.GetOrderResult{}, fmt.Errorf("order %s did not reach terminal status within %s", clientOrderID, timeout)
}

// RequireTradingBaseBalanceForSymbol skips when base trading balance is below qty.
func RequireTradingBaseBalanceForSymbol(t *testing.T, ctx context.Context, client *polyester.Client, symbol, qty string) {
	t.Helper()
	if SkipFundingCheck() {
		return
	}
	spotRaw := spotRawFromClient(t, client, ctx)
	zipper := CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	baseAssetID := BaseAssetIDForSymbol(spotRaw, symbol, zipper)
	if baseAssetID == nil {
		t.Skipf("cannot resolve base asset for %s", symbol)
	}
	qtyDecimal, err := DecimalStringRequired(qty)
	if err != nil {
		t.Fatal(err)
	}
	balances := CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	balance := TradingBalanceHuman(balances, *baseAssetID)
	if balance.Cmp(qtyDecimal) < 0 {
		t.Skipf("trading base balance %s below required %s for asset %d; fund devnet or set POLYESTER_TEST_SKIP_FUNDING_CHECK=1",
			balance.FloatString(8), qtyDecimal.FloatString(8), *baseAssetID)
	}
}
