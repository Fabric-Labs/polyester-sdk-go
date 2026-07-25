//go:build integration

package integration_test

import (
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func requireChainUserOp(t *testing.T) string {
	t.Helper()
	if !testutil.EnvTruthy("POLYESTER_TEST_CHAIN_USEROP") {
		t.Skip("Set POLYESTER_TEST_CHAIN_USEROP=1 to run on-chain Funding UserOp tests")
	}
	owner := strings.TrimSpace(os.Getenv("POLYESTER_OWNER_PRIVATE_KEY"))
	if owner == "" {
		t.Skip("Set POLYESTER_OWNER_PRIVATE_KEY for on-chain Funding UserOp tests")
	}
	return owner
}

func usdtAsset(t *testing.T, cfg models.DepositWithdrawConfig) models.ZipperAssetConfig {
	t.Helper()
	override := strings.TrimSpace(os.Getenv("POLYESTER_TEST_U_ASSET_ID"))
	for _, asset := range cfg.Assets {
		if override != "" && strings.EqualFold(asset.UAssetID, override) {
			return asset
		}
		if asset.LedgerID == 1 || strings.EqualFold(asset.Asset, "USDT") {
			return asset
		}
	}
	t.Skip("USDT / ledger_id=1 not found in zipper deposit-withdraw config")
	return models.ZipperAssetConfig{}
}

func depositQtyScaled(t *testing.T) *big.Int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("POLYESTER_TEST_DEPOSIT_QTY_SCALED"))
	if raw == "" {
		return new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 1 USDT
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok || n.Sign() <= 0 {
		t.Fatalf("invalid POLYESTER_TEST_DEPOSIT_QTY_SCALED=%q", raw)
	}
	return n
}

func TestFundingToTradingUserOp(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)
	owner := requireChainUserOp(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	cfg := testutil.CallRequired(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	asset := usdtAsset(t, cfg)
	qty := depositQtyScaled(t)

	before := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	fundingBefore := testutil.FundingBalance(before, asset.LedgerID)
	tradingBefore := testutil.TradingBalance(before, asset.LedgerID)
	if fundingBefore.Cmp(qty) < 0 {
		t.Skipf("funding balance %s below deposit quantity %s for asset %d", fundingBefore, qty, asset.LedgerID)
	}

	account, err := chain.NewSmartAccount(owner, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call, err := chain.EncodeTradingGatewayDeposit(
		chain.PolyesterTestnetEnvironment.Contracts.TradingGatewayAddress,
		asset.UAssetID,
		qty,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := account.SendCalls([]chain.ChainCall{call}, true, 120*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if receipt == nil || !receipt.Success || receipt.UserOperationHash == "" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	wantTrading := new(big.Int).Add(tradingBefore, qty)
	tradingAfter := tradingBefore
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		after := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
			return client.Balances.List(ctx, nil, nil)
		})
		tradingAfter = testutil.TradingBalance(after, asset.LedgerID)
		if tradingAfter.Cmp(wantTrading) >= 0 {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("trading after=%s want >= %s", tradingAfter, wantTrading)
}

func TestFundingWithdrawToChainUserOp(t *testing.T) {
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)
	owner := requireChainUserOp(t)
	dest := strings.TrimSpace(os.Getenv("POLYESTER_TEST_WITHDRAW_DESTINATION"))
	if dest == "" {
		t.Skip("Set POLYESTER_TEST_WITHDRAW_DESTINATION for Funding→external UserOp")
	}
	chainIDRaw := strings.TrimSpace(os.Getenv("POLYESTER_TEST_WITHDRAW_CHAIN_ID"))
	if chainIDRaw == "" {
		chainIDRaw = "6"
	}
	chainID64, err := strconv.ParseUint(chainIDRaw, 10, 16)
	if err != nil || chainID64 == 0 {
		t.Fatalf("invalid POLYESTER_TEST_WITHDRAW_CHAIN_ID=%q", chainIDRaw)
	}
	chainID := uint16(chainID64)

	humanAmount := strings.TrimSpace(os.Getenv("POLYESTER_TEST_WITHDRAW_AMOUNT"))
	if humanAmount == "" {
		humanAmount = "1"
	}
	zAmountRat, ok := new(big.Rat).SetString(humanAmount)
	if !ok {
		t.Fatalf("invalid POLYESTER_TEST_WITHDRAW_AMOUNT=%q", humanAmount)
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	zAmount := new(big.Int).Quo(new(big.Int).Mul(zAmountRat.Num(), scale.Num()), zAmountRat.Denom())
	if zAmount.Sign() <= 0 {
		t.Fatalf("withdraw amount must be > 0, got %s", zAmount)
	}

	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	cfg := testutil.CallRequired(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	asset := usdtAsset(t, cfg)
	var variant *models.ZipperAssetChainVariant
	for i := range asset.Variants {
		if asset.Variants[i].ChainID == uint32(chainID) && asset.Variants[i].ZToken.Address != "" {
			variant = &asset.Variants[i]
			break
		}
	}
	if variant == nil {
		t.Skipf("No USDT z_token variant for withdraw chain_id=%d", chainID)
	}
	caseSensitive := false
	for _, c := range cfg.Chains {
		if c.ChainID == uint32(chainID) {
			caseSensitive = c.IsCaseSensitive
			break
		}
	}

	before := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
		return client.Balances.List(ctx, nil, nil)
	})
	fundingBefore := testutil.FundingBalance(before, asset.LedgerID)
	if fundingBefore.Cmp(zAmount) < 0 {
		t.Skipf("funding balance %s below withdraw amount %s for asset %d", fundingBefore, zAmount, asset.LedgerID)
	}

	fee, err := chain.QuoteZipperFee(
		chainID,
		variant.ZToken.Address,
		chain.PolyesterTestnetEnvironment.Contracts.ZipperEndpointAddress,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	maxFee := new(big.Int).Add(fee.Fee, new(big.Int).Div(fee.Fee, big.NewInt(10)))
	if zAmount.Cmp(maxFee) <= 0 {
		t.Skipf("withdraw amount %s must be greater than max_fee %s; raise POLYESTER_TEST_WITHDRAW_AMOUNT", zAmount, maxFee)
	}

	account, err := chain.NewSmartAccount(owner, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call, err := chain.EncodeFundingWithdrawToChain(
		chain.PolyesterTestnetEnvironment.Contracts.FundingAccountAddress,
		chainID,
		variant.ZToken.Address,
		chain.EncodeWithdrawDestination(dest, caseSensitive),
		zAmount,
		maxFee,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := account.SendCalls([]chain.ChainCall{call}, true, 120*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if receipt == nil || !receipt.Success || receipt.UserOperationHash == "" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	fundingAfter := fundingBefore
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		after := testutil.CallRequired(t, "balances.list", func() (models.BalancesList, error) {
			return client.Balances.List(ctx, nil, nil)
		})
		fundingAfter = testutil.FundingBalance(after, asset.LedgerID)
		if fundingAfter.Cmp(fundingBefore) < 0 {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("funding after=%s did not decrease from %s", fundingAfter, fundingBefore)
}
