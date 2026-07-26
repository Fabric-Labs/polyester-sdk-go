//go:build integration

package integration_test

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestTransferToUserTiny(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)

	bucket := strings.ToLower(strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRANSFER_SOURCE_BUCKET")))
	if bucket == "" {
		bucket = "funding"
	}
	if bucket == "funding" {
		t.Skip(
			"Funding→another user is on-chain in the Polyester app " +
				"(FundingAccount.UAssetTransfer + wallet/smart-account signing), not an API-key RPC. " +
				"Set POLYESTER_TEST_TRANSFER_SOURCE_BUCKET=unified to run unified→user via internal_transfers.Create, " +
				"or run TestInternalTransferTiny.",
		)
	}
	if bucket != "unified" {
		t.Skipf("unknown POLYESTER_TEST_TRANSFER_SOURCE_BUCKET=%q; use funding or unified", bucket)
	}

	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	dest := testutil.InternalTransferDest()
	if dest == "" {
		t.Skip("Set POLYESTER_TEST_INTERNAL_TRANSFER_DEST for internal transfer e2e")
	}

	symbol := testutil.SmokeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

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
		t.Skipf("cannot resolve quote asset for internal transfer on %s", symbol)
	}

	quantity := os.Getenv("POLYESTER_TEST_INTERNAL_TRANSFER_QTY")
	if quantity == "" {
		quantity = "1"
	}
	qtyScaled, err := testutil.ScaledQuantityString(quantity, codecs.LedgerScale)
	if err != nil {
		t.Fatal(err)
	}
	qtyInt, ok := new(big.Int).SetString(qtyScaled, 10)
	if !ok {
		t.Fatalf("parse scaled quantity %q", qtyScaled)
	}

	before := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	tradingBefore := testutil.TradingBalance(before, *assetID)
	if tradingBefore.Cmp(qtyInt) < 0 {
		t.Skipf("trading balance %s below transfer quantity %s for asset %d", tradingBefore, quantity, *assetID)
	}

	idempotencyKey := testutil.UniqueClientOrderID("e2e-xfer")
	result, err := client.InternalTransfers.Create(ctx, *assetID, models.AssetAmountFromDecimal(quantity), idempotencyKey, nil, nil, &dest, nil, nil, nil)
	if err != nil {
		if testutil.DevnetBackendUnavailable(err) || testutil.RouteUnavailable(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet internal transfer unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if result.RequestID == "" && result.TransferID == "" {
		t.Fatal("expected request_id or transfer_id")
	}
	expectedScaled, err := testutil.ScaledQuantityString(quantity, codecs.LedgerScale)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(result.Quantity.Scaled) != expectedScaled {
		t.Fatalf("quantity=%v want %q", result.Quantity.Scaled, expectedScaled)
	}

	expectedAfter := new(big.Int).Sub(tradingBefore, qtyInt)
	tradingAfter := tradingBefore
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		after := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
			return client.Balances.List(ctx, nil, nil)
		})
		tradingAfter = testutil.TradingBalance(after, *assetID)
		if tradingAfter.Cmp(expectedAfter) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if tradingAfter.Cmp(expectedAfter) != 0 {
		t.Fatalf("trading balance after=%s want %s", tradingAfter, expectedAfter)
	}
}
