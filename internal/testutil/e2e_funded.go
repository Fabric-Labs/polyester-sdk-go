package testutil

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// MakerClientFromEnv returns a second client when maker API key env vars are set.
func MakerClientFromEnv() (*polyester.Client, bool, error) {
	loadDotEnv()
	keyID := strings.TrimSpace(os.Getenv("POLYESTER_TEST_MAKER_API_KEY_ID"))
	privateKey := strings.TrimSpace(os.Getenv("POLYESTER_TEST_MAKER_API_PRIVATE_KEY"))
	if keyID == "" || privateKey == "" {
		return nil, false, nil
	}
	cfg := polyester.Config{
		APIKeyID:        keyID,
		APIPrivateKey:   privateKey,
		HydrateCatalogs: true,
	}
	if apiURL := strings.TrimSpace(os.Getenv("POLYESTER_API_URL")); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if wsURL := strings.TrimSpace(os.Getenv("POLYESTER_WS_URL")); wsURL != "" {
		cfg.WSURL = wsURL
	}
	client, err := polyester.New(cfg)
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}

// IsNotFound reports API not_found responses used by polling helpers.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		return strings.EqualFold(strings.TrimSpace(api.Code), "not_found")
	}
	return false
}

// ScaledQuantityString converts a decimal quantity to scaled ledger units.
func ScaledQuantityString(quantity string, scale int) (string, error) {
	qty, ok := new(big.Rat).SetString(quantity)
	if !ok {
		return "", fmt.Errorf("invalid quantity %q", quantity)
	}
	multiplier := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	scaled := new(big.Rat).Mul(qty, multiplier)
	if !scaled.IsInt() {
		return "", fmt.Errorf("quantity %q does not scale cleanly to %d decimals", quantity, scale)
	}
	return scaled.Num().String(), nil
}

// DecimalStringRequired parses a required positive decimal string.
func DecimalStringRequired(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty decimal")
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", value)
	}
	return rat, nil
}

// WaitForFilledOrder polls until an order is filled with trades.
func WaitForFilledOrder(ctx context.Context, client *polyester.Client, clientOrderID string, timeout time.Duration) (models.GetOrderResult, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var last models.GetOrderResult
	for time.Now().Before(deadline) {
		detail, err := client.Orders.Get(ctx, nil, nil, &clientOrderID, nil, false, false)
		if err != nil {
			return models.GetOrderResult{}, err
		}
		last = detail
		if detail.Order != nil && detail.Order.Status == "filled" && len(detail.Trades) > 0 {
			return detail, nil
		}
		select {
		case <-ctx.Done():
			return models.GetOrderResult{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return last, fmt.Errorf("order %s did not fill within %s", clientOrderID, timeout)
}

// WaitForTradeMatch polls user trades until match_id appears.
func WaitForTradeMatch(ctx context.Context, client *polyester.Client, symbol, matchID string, timeout time.Duration) (models.UserTrade, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		trades, err := client.Trades.List(ctx, nil, nil, &symbol, nil, 25, nil)
		if err != nil {
			return models.UserTrade{}, err
		}
		for _, trade := range trades.Trades {
			if trade.MatchID == matchID {
				return trade, nil
			}
		}
		select {
		case <-ctx.Done():
			return models.UserTrade{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return models.UserTrade{}, fmt.Errorf("user trade %s was not visible within %s", matchID, timeout)
}

// BestAskParams returns price and qty to buy at the current best ask.
func BestAskParams(ctx context.Context, client *polyester.Client, symbol string, pair map[string]any) (price, qty string, err error) {
	book, err := client.Orderbook.Get(ctx, symbol, 5)
	if err != nil {
		if RouteUnavailable(err) {
			return "", "", err
		}
		return "", "", err
	}
	if len(book.Asks) == 0 {
		return "", "", fmt.Errorf("no visible asks on %s", symbol)
	}
	best := book.Asks[0]
	minTicks := best.Price.Ticks()
	for _, ask := range book.Asks[1:] {
		ticks := ask.Price.Ticks()
		if ticks < minTicks {
			minTicks = ticks
			best = ask
		}
	}
	price = codecs.FormatPriceTicks(minTicks)
	if override := strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRADE_QTY")); override != "" {
		return price, override, nil
	}
	qty = MinBaseQtyForPair(pair, price)
	qtyFmt, err := best.Qty.Format()
	if err != nil {
		return "", "", err
	}
	available, err := DecimalStringRequired(qtyFmt)
	if err != nil {
		return "", "", err
	}
	requested, err := DecimalStringRequired(qty)
	if err != nil {
		return "", "", err
	}
	if available.Cmp(requested) < 0 {
		return "", "", fmt.Errorf("best ask quantity %s below requested %s on %s", qtyFmt, qty, symbol)
	}
	return price, qty, nil
}
