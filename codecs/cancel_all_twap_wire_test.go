package codecs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestCancelAllOrdersToProtoUsesRepeatedSymbolIDs(t *testing.T) {
	proto, err := CancelAllOrdersToProto(nil, []uint32{3, 3, 7}, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.GetDryRun() {
		t.Fatal("dry_run")
	}
	if got := proto.GetSymbolIds(); len(got) != 2 || got[0] != 3 || got[1] != 7 {
		t.Fatalf("symbol_ids=%v", got)
	}
	if proto.SubaccountId != nil {
		t.Fatal("subaccount should be omitted")
	}
}

func TestCancelAllOrdersToProtoEmptyMatchesAllSymbols(t *testing.T) {
	proto, err := CancelAllOrdersToProto(nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(proto.GetSymbolIds()) != 0 {
		t.Fatalf("symbol_ids=%v", proto.GetSymbolIds())
	}
}

func TestCancelAllOrdersToProtoRejectsZeroAndOverLimit(t *testing.T) {
	if _, err := CancelAllOrdersToProto(nil, []uint32{0}, nil, false, nil); err == nil {
		t.Fatal("expected positive-id validation error")
	}
	ids := make([]uint32, 101)
	for i := range ids {
		ids[i] = uint32(i + 1)
	}
	if _, err := CancelAllOrdersToProto(nil, ids, nil, false, nil); err == nil {
		t.Fatal("expected 100-id limit validation error")
	}
}

func TestTwapMarketIocEncodesOptionalSlippage(t *testing.T) {
	bps := int32(25)
	duration := int64(60_000)
	interval := int64(5_000)
	req, err := CreateTriggerToProto(
		1, "BTC-USDT", "twap", nil, "buy", models.QtyFromDecimal("1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TwapDurationMs: &duration, TwapSliceIntervalMs: &interval, MaxSlippageBps: &bps},
	)
	if err != nil {
		t.Fatal(err)
	}
	ioc := req.GetTrigger().GetTwap().GetMarketIoc()
	if ioc == nil {
		t.Fatal("market_ioc missing")
	}
	if ioc.GetMaxSlippageBps() != 25 {
		t.Fatalf("max_slippage_bps=%d", ioc.GetMaxSlippageBps())
	}

	invalid := int32(10_001)
	_, err = CreateTriggerToProto(
		1, "BTC-USDT", "twap", nil, "buy", models.QtyFromDecimal("1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{MaxSlippageBps: &invalid},
	)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
}
