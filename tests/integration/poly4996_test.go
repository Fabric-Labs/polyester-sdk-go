//go:build integration

package integration_test

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

const (
	deadSmartAccount      = "0x000000000000000000000000000000000000dEaD"
	impossibleWithdrawQty = "999999999"
)

func assertOptionalScaled(t *testing.T, value *string, label string) {
	t.Helper()
	if value == nil || *value == "" {
		return
	}
	parsed, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		t.Fatalf("%s is not a number: %q (%v)", label, *value, err)
	}
	if parsed < 0 {
		t.Fatalf("%s must be non-negative: %q", label, *value)
	}
}

func TestSpotVolumeHistoryBySymbol(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallOptional(t, "market_overview.get_spot_volume_history", func() (models.SpotVolumeHistory, error) {
		return client.MarketOverview.GetSpotVolumeHistory(ctx, []string{symbol}, nil)
	})
	if result.Points > 0 && len(result.TotalVolumeUsdScaled) != int(result.Points) {
		t.Fatalf("total_volume_usd_scaled=%d points=%d", len(result.TotalVolumeUsdScaled), result.Points)
	}
	if result.EndTsSec < result.StartTsSec {
		t.Fatalf("end_ts_sec=%d start_ts_sec=%d", result.EndTsSec, result.StartTsSec)
	}
	wantID := client.Catalogs.SymbolIDForSymbol(symbol)
	for _, pair := range result.Pairs {
		if pair.SymbolID == 0 {
			t.Fatalf("pair missing symbol_id: %+v", pair)
		}
		if len(pair.VolumeUsdScaled) != int(result.Points) {
			t.Fatalf("pair %d volume samples=%d points=%d", pair.SymbolID, len(pair.VolumeUsdScaled), result.Points)
		}
		if wantID != nil && pair.SymbolID != *wantID {
			t.Fatalf("pair symbol_id=%d want %d", pair.SymbolID, *wantID)
		}
	}
}

func TestSpotVolumeHistoryBySymbolID(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	symbolID := client.Catalogs.SymbolIDForSymbol(symbol)
	if symbolID == nil {
		t.Skipf("catalog missing symbol_id for %s", symbol)
	}
	result := testutil.CallOptional(t, "market_overview.get_spot_volume_history(symbol_ids)", func() (models.SpotVolumeHistory, error) {
		return client.MarketOverview.GetSpotVolumeHistory(ctx, nil, []uint32{*symbolID})
	})
	for _, pair := range result.Pairs {
		if pair.SymbolID != *symbolID {
			t.Fatalf("pair symbol_id=%d want %d", pair.SymbolID, *symbolID)
		}
	}
}

func TestCandlesPreserveQuoteVolume(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	result := testutil.CallRequired(t, "market_data.get_candles", func() (models.CandlesResult, error) {
		return client.MarketData.GetCandles(ctx, &symbol, nil, "1m", 5, nil, nil, false)
	})
	for _, candle := range result.Candles {
		if candle.QuoteVolume != "" {
			parsed, err := strconv.ParseFloat(candle.QuoteVolume, 64)
			if err != nil {
				t.Fatalf("quote_volume %q: %v", candle.QuoteVolume, err)
			}
			if parsed < 0 {
				t.Fatalf("quote_volume must be non-negative: %q", candle.QuoteVolume)
			}
		}
	}
}

func TestMarketOverviewOptionalVolumeFields(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "market_overview.list", func() (models.MarketOverviewList, error) {
		return client.MarketOverview.List(ctx, nil, 10, "", false)
	})
	for _, market := range result.Markets {
		if market.SymbolID == 0 {
			t.Fatalf("market missing symbol_id: %+v", market)
		}
		assertOptionalScaled(t, market.Volume24HBaseScaled, "volume_24h_base_scaled")
		assertOptionalScaled(t, market.Volume24HQuoteScaled, "volume_24h_quote_scaled")
		assertOptionalScaled(t, market.Volume24HUsdScaled, "volume_24h_usd_scaled")
	}
}

func TestCancelAllDryRunSymbolsAndSymbolIDs(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	symbolID := client.Catalogs.SymbolIDForSymbol(symbol)
	if symbolID == nil {
		t.Skipf("catalog missing symbol_id for %s", symbol)
	}

	bySymbols := testutil.CallRequired(t, "orders.cancel_all(symbols)", func() (models.CancelAllOrdersResult, error) {
		return client.Orders.CancelAllWith(ctx, nil, services.CancelAllParams{
			Symbols: []string{symbol},
			DryRun:  true,
		})
	})
	if bySymbols.Status == "" {
		t.Fatalf("expected status: %+v", bySymbols)
	}
	if bySymbols.SubmittedCancels != 0 {
		t.Fatalf("submitted_cancels=%d want 0 for dry_run", bySymbols.SubmittedCancels)
	}

	byIDs := testutil.CallRequired(t, "orders.cancel_all(symbol_ids)", func() (models.CancelAllOrdersResult, error) {
		return client.Orders.CancelAllWith(ctx, nil, services.CancelAllParams{
			SymbolIDs: []uint32{*symbolID},
			DryRun:    true,
		})
	})
	if byIDs.Status == "" {
		t.Fatalf("expected status: %+v", byIDs)
	}
	if byIDs.SubmittedCancels != 0 {
		t.Fatalf("submitted_cancels=%d want 0 for dry_run", byIDs.SubmittedCancels)
	}
}

func TestTriggerListDecodesLadderExecutedFields(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "triggers.list", func() (models.TriggersList, error) {
		return client.Triggers.List(ctx, nil, nil, nil, nil, 20, nil)
	})
	for _, trigger := range result.Triggers {
		if trigger.Details == nil || trigger.Details.Case != "ladder" {
			continue
		}
		if trigger.Details.ExecutedLevels < 0 {
			t.Fatalf("executed_levels=%d", trigger.Details.ExecutedLevels)
		}
	}
}

func TestCreateLadderDecodesExecutedFields(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	price, qty, err := testutil.USDTFundedBuyLimitParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	maxTicks, err := codecs.ParsePriceTicks(price, "ladder_price_max")
	if err != nil {
		t.Fatal(err)
	}
	tick := int64(1)
	if client.Catalogs != nil {
		if constraints, ok := client.Catalogs.PairConstraintsForSymbol(symbol); ok && constraints.TickSizeTicks > 0 {
			tick = constraints.TickSizeTicks
		}
	}
	minTicks := int64(maxTicks) * 8 / 10 / tick * tick
	if minTicks < tick {
		minTicks = tick
	}
	if minTicks <= 0 || minTicks >= int64(maxTicks) {
		t.Skip("could not form a valid far-below ladder range")
	}
	qtyRat, ok := new(big.Rat).SetString(qty)
	if !ok {
		t.Fatalf("invalid qty %q", qty)
	}
	qtyRat.Mul(qtyRat, big.NewRat(2, 1))
	totalQty := qtyRat.FloatString(16)
	minPrice := models.PriceFromTicksInt(minTicks)
	maxPrice := models.PriceFromTicksInt(int64(maxTicks))
	levels := int32(2)
	dist := "linear"
	clientID := testutil.UniqueClientOrderID("trg-ladder-exec")
	created, createErr := client.Triggers.Create(
		ctx, nil, symbol, "ladder", nil, "buy", models.QtyFromDecimal(totalQty), "limit",
		&maxPrice, "", "", nil, &clientID, false, codecs.CreateTriggerOptions{
			LadderPriceMin:     &minPrice,
			LadderPriceMax:     &maxPrice,
			LadderLevels:       &levels,
			LadderDistribution: &dist,
		},
	)
	if createErr != nil {
		if testutil.DevnetBackendUnavailable(createErr) {
			t.Skipf("devnet trigger placement unavailable: %v", createErr)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(createErr, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("trigger sizing below min notional: %v", createErr)
		}
		t.Fatal(createErr)
	}
	if created.TriggerID == "" || created.Status != "accepted" {
		t.Fatalf("create result=%+v", created)
	}
	defer func() {
		_, _ = client.Triggers.Cancel(ctx, nil, created.TriggerID, nil)
	}()

	got, getErr := testutil.WaitForTrigger(ctx, client, created.TriggerID, 0)
	if getErr != nil {
		t.Fatalf("triggers.get: %v", getErr)
	}
	if got.Details == nil || got.Details.Case != "ladder" {
		t.Fatalf("expected ladder details, got %+v", got)
	}
	if got.Details.ExecutedLevels < 0 {
		t.Fatalf("executed_levels=%d", got.Details.ExecutedLevels)
	}
}

func assertTypedMoneyError(t *testing.T, err error, label string) {
	t.Helper()
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		if api.Code == "" {
			t.Fatalf("%s missing ErrorDetail code: %v", label, err)
		}
		if !strings.HasPrefix(api.Code, "ERROR_CODE_") && !strings.HasPrefix(api.Code, "UNKNOWN_ERROR_CODE") {
			t.Fatalf("%s unexpected code=%q err=%v", label, api.Code, err)
		}
		return
	}
	var auth *sdkerrors.AuthError
	if errors.As(err, &auth) {
		return
	}
	if testutil.RouteUnavailable(err) {
		t.Skipf("%s not mounted on devnet: %v", label, err)
	}
	t.Fatalf("%s did not raise a mapped SDK error: %v", label, err)
}

func TestTwapMarketIocAcceptsSlippage(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	price, qty, err := testutil.USDTFundedBuyLimitParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	_ = price
	createAndCancel := func(opts codecs.CreateTriggerOptions, clientID string) {
		t.Helper()
		created, createErr := client.Triggers.Create(
			ctx, nil, symbol, "twap", nil, "buy", models.QtyFromDecimal(qty), "market",
			nil, "", "", nil, &clientID, false, opts,
		)
		if createErr != nil {
			if testutil.DevnetBackendUnavailable(createErr) {
				t.Skipf("devnet trigger placement unavailable: %v", createErr)
			}
			var validation *sdkerrors.ValidationError
			if errors.As(createErr, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
				t.Skipf("trigger sizing below min notional: %v", createErr)
			}
			t.Fatal(createErr)
		}
		if created.TriggerID == "" || created.Status != "accepted" {
			t.Fatalf("create result=%+v", created)
		}
		_, _ = client.Triggers.Cancel(ctx, nil, created.TriggerID, nil)
	}

	bps := int32(25)
	ticks := int32(1)
	duration := int64(600_000)
	interval := int64(300_000)
	createAndCancel(codecs.CreateTriggerOptions{
		MaxSlippageBps:      &bps,
		TwapDurationMs:      &duration,
		TwapSliceIntervalMs: &interval,
	}, testutil.UniqueClientOrderID("trg-twap-bps"))
	createAndCancel(codecs.CreateTriggerOptions{
		MaxSlippageTicks:    &ticks,
		TwapDurationMs:      &duration,
		TwapSliceIntervalMs: &interval,
	}, testutil.UniqueClientOrderID("trg-twap-ticks"))
}

func TestSocialVerificationDiscordWithoutHandle(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "social_verification.start(discord)", func() (models.ApiData, error) {
		return client.SocialVerification.Start(ctx, "discord", "", "")
	})
	_ = result
	_ = testutil.CallOptional(t, "social_verification.get(discord)", func() (models.ApiData, error) {
		return client.SocialVerification.Get(ctx, "discord")
	})
}

func TestWithdrawErrorDetailFromImpossibleFundingMove(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	spotRaw := testutil.CallRequired(t, "market_data.get_spot_config", func() (models.SpotConfig, error) {
		return client.MarketData.GetSpotConfig(ctx)
	}).Raw
	if client.Catalogs != nil {
		_ = client.Catalogs.HydrateSpotConfig(spotRaw)
	}
	zipper := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	assetID := testutil.QuoteAssetIDForSymbol(spotRaw, symbol, zipper)
	if assetID == nil {
		t.Skipf("cannot resolve quote asset for %s", symbol)
	}
	_, err := client.Withdraw.CreateAPIKeyToFunding(ctx, services.PrepareAPIKeyWithdrawParams{
		AssetID:        *assetID,
		Amount:         models.AssetAmountFromDecimal(impossibleWithdrawQty),
		IdempotencyKey: testutil.UniqueClientOrderID("poly4996-wd"),
	})
	if err == nil {
		t.Fatal("expected withdraw ErrorDetail for an impossible funding amount")
	}
	assertTypedMoneyError(t, err, "withdraw.CreateAPIKeyToFunding")
}

func TestTransferErrorDetailFromUnknownDestination(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	spotRaw := testutil.CallRequired(t, "market_data.get_spot_config", func() (models.SpotConfig, error) {
		return client.MarketData.GetSpotConfig(ctx)
	}).Raw
	if client.Catalogs != nil {
		_ = client.Catalogs.HydrateSpotConfig(spotRaw)
	}
	zipper := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	assetID := testutil.QuoteAssetIDForSymbol(spotRaw, symbol, zipper)
	if assetID == nil {
		t.Skipf("cannot resolve quote asset for %s", symbol)
	}
	dest := deadSmartAccount
	_, err := client.InternalTransfers.Create(
		ctx, *assetID, models.AssetAmountFromDecimal("0.000001"),
		testutil.UniqueClientOrderID("poly4996-xfer"), nil, nil, nil, nil, &dest, nil,
	)
	if err == nil {
		t.Fatal("expected transfer ErrorDetail for an unknown destination")
	}
	assertTypedMoneyError(t, err, "internal_transfers.Create")
}
