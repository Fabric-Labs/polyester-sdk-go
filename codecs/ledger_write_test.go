package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
)

func TestTransferTradingToTradingToProto(t *testing.T) {
	req, err := codecs.TransferTradingToTradingToProto(1, 2, 3, "1.0", nil, 18)
	if err != nil {
		t.Fatal(err)
	}
	if req.GetFromAccountId() != 1 || req.GetToAccountId() != 2 || req.GetLedger() != 3 {
		t.Fatalf("req=%+v", req)
	}
	if req.GetRequestId() == "" || req.GetAmountUnits() == nil {
		t.Fatal("expected request id and amount units")
	}
}

func TestReleaseTradingWithdrawReserveRequiresIntentID(t *testing.T) {
	if _, err := codecs.ReleaseTradingWithdrawReserveToProto(1, 2, "", ""); err == nil {
		t.Fatal("expected validation error")
	}
}
