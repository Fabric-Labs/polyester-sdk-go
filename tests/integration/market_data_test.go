//go:build integration

package integration_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestGetSpotConfig(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "market_data.get_spot_config", func() (models.SpotConfig, error) {
		return client.MarketData.GetSpotConfig(ctx)
	})
	pairs, _ := result.Raw["pairs"].([]any)
	if len(pairs) == 0 {
		t.Fatal("expected spot pairs")
	}
	for _, item := range pairs {
		pair, _ := item.(map[string]any)
		if pair["symbol"] == "" {
			t.Fatalf("pair missing symbol: %+v", pair)
		}
	}
}

func TestGetTrades(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "market_data.get_trades", func() (models.MarketTradesResult, error) {
		return client.MarketData.GetTrades(ctx, &symbol, nil, 5, nil)
	})
	for _, trade := range result.Trades {
		if trade.SymbolID == 0 || trade.MatchID == "" {
			t.Fatalf("trade missing ids: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, fmt.Sprint(trade.Price.Ticks)).Sign() == 0 {
			t.Fatalf("trade price_ticks must be positive: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, fmt.Sprint(trade.Qty.Scaled)).Sign() == 0 {
			t.Fatalf("trade qty_scaled must be positive: %+v", trade)
		}
		if testutil.NonNegativeIntStringPositive(t, trade.TsNs).Sign() == 0 {
			t.Fatalf("trade ts_ns must be positive: %+v", trade)
		}
	}
}

func TestGetCandles(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "market_data.get_candles", func() (models.CandlesResult, error) {
		return client.MarketData.GetCandles(ctx, &symbol, nil, "1m", 5, nil, nil, false)
	})
	for _, candle := range result.Candles {
		if candle.TsSec < 0 {
			t.Fatalf("candle ts_sec=%d", candle.TsSec)
		}
		high, err := strconv.ParseFloat(candle.High, 64)
		if err != nil {
			t.Fatalf("parse high %q: %v", candle.High, err)
		}
		low, err := strconv.ParseFloat(candle.Low, 64)
		if err != nil {
			t.Fatalf("parse low %q: %v", candle.Low, err)
		}
		if high < low {
			t.Fatalf("high=%v low=%v", high, low)
		}
	}
}

func TestGetCurrentCandleOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	candle := testutil.CallOptional(t, "market_data.get_current_candle", func() (models.Candle, error) {
		return client.MarketData.GetCurrentCandle(ctx, &symbol, nil, "1m")
	})
	if candle.TsSec < 0 {
		t.Fatalf("candle ts_sec=%d", candle.TsSec)
	}
}
