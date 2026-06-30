//go:build integration

package integration_test

import (
	"os"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestLedgerWriteTransferTradingToTradingOptional(t *testing.T) {
	if !testutil.EnvTruthy("POLYESTER_TEST_LEDGER_WRITE_SMOKE") {
		t.Skip("Set POLYESTER_TEST_LEDGER_WRITE_SMOKE=1 to probe ledger_write mutations")
	}
	toAccount := os.Getenv("POLYESTER_TEST_INTERNAL_TRANSFER_DEST")
	if toAccount == "" {
		t.Skip("POLYESTER_TEST_INTERNAL_TRANSFER_DEST required for ledger_write smoke")
	}

	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	zipper := testutil.CallRequired(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	if len(zipper.Assets) == 0 {
		t.Skip("zipper config missing assets")
	}
	ledgerID := int(zipper.Assets[0].LedgerID)
	if ledgerID <= 0 {
		t.Skip("cannot resolve ledger id")
	}

	result := testutil.CallOptional(t, "ledger_write.transfer_trading_to_trading", func() (models.LedgerWriteTransferResult, error) {
		return client.LedgerWrite.TransferTradingToTrading(ctx, toAccount, ledgerID, "0.00000001", nil, nil, 18)
	})
	if result.TransferID == "" {
		t.Fatalf("result=%+v", result)
	}
}
