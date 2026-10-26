package codecs

import (
	"errors"
	"math"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func createConditional(triggerType, side, orderType string, limitPrice *models.PriceInput, opts CreateTriggerOptions) (*triggersv1.CreateTriggerRequest, error) {
	trigger := models.PriceFromDecimal("60000")
	return CreateTriggerToProto(
		1, "BTC-USDT", triggerType, &trigger, side, models.QtyFromDecimal("0.1"),
		orderType, limitPrice, "", "gtc", nil, nil, false, 8, opts,
	)
}

func wantValidation(t *testing.T, err error, contains string) {
	t.Helper()
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
	if !strings.Contains(validationErr.Msg, contains) {
		t.Fatalf("error %q does not mention %q", validationErr.Msg, contains)
	}
}

func conditionalOf(req *triggersv1.CreateTriggerRequest) *triggersv1.ConditionalTrigger {
	if c := req.GetTrigger().GetStopLoss(); c != nil {
		return c
	}
	return req.GetTrigger().GetTakeProfit()
}

// The cap a caller sets on a stop-loss or take-profit market child must reach
// the wire: dropping it would let the child fill at the pair's default
// slippage instead of the caller's limit.
func TestConditionalMarketChildCarriesSlippageCap(t *testing.T) {
	ticks := int64(250_000_000_000)
	bps := int32(75)
	for _, triggerType := range []string{"stop_loss", "take_profit"} {
		for _, side := range []string{"buy", "sell"} {
			req, err := createConditional(triggerType, side, "market", nil, CreateTriggerOptions{MaxSlippageTicks: &ticks})
			if err != nil {
				t.Fatalf("%s %s ticks: %v", triggerType, side, err)
			}
			ioc := conditionalOf(req).GetChild().GetMarketIoc()
			if ioc == nil || ioc.GetMaxSlippageTicks() != ticks {
				t.Fatalf("%s %s: market child %+v, want max_slippage_ticks=%d", triggerType, side, ioc, ticks)
			}

			req, err = createConditional(triggerType, side, "market", nil, CreateTriggerOptions{MaxSlippageBps: &bps})
			if err != nil {
				t.Fatalf("%s %s bps: %v", triggerType, side, err)
			}
			if got := conditionalOf(req).GetChild().GetMarketIoc().GetMaxSlippageBps(); got != bps {
				t.Fatalf("%s %s: max_slippage_bps=%d, want %d", triggerType, side, got, bps)
			}
		}
	}

	// Without a cap the pair default applies: the oneof stays unset.
	req, err := createConditional("stop_loss", "buy", "market", nil, CreateTriggerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ioc := conditionalOf(req).GetChild().GetMarketIoc(); ioc == nil || ioc.GetMaxSlippage() != nil {
		t.Fatalf("uncapped market child %+v", ioc)
	}
}

func TestConditionalSlippageCapValidation(t *testing.T) {
	ticks := int64(1)
	bps := int32(10)
	_, err := createConditional("stop_loss", "sell", "market", nil, CreateTriggerOptions{MaxSlippageTicks: &ticks, MaxSlippageBps: &bps})
	wantValidation(t, err, "at most one of max_slippage_ticks or max_slippage_bps")

	zero := int64(0)
	_, err = createConditional("stop_loss", "sell", "market", nil, CreateTriggerOptions{MaxSlippageTicks: &zero})
	wantValidation(t, err, "max_slippage_ticks must be positive")

	full := int32(10_000)
	if _, err := createConditional("take_profit", "buy", "market", nil, CreateTriggerOptions{MaxSlippageBps: &full}); err != nil {
		t.Fatalf("buy market child accepts 10000 bps: %v", err)
	}
	_, err = createConditional("take_profit", "sell", "market", nil, CreateTriggerOptions{MaxSlippageBps: &full})
	wantValidation(t, err, "between 1 and 9999")

	limit := models.PriceFromDecimal("59000")
	_, err = createConditional("stop_loss", "sell", "limit", &limit, CreateTriggerOptions{MaxSlippageBps: &bps})
	wantValidation(t, err, "apply only to market children")
}

// The contract no longer requires a buy stop-loss or take-profit to use a limit child.
func TestBuyConditionalAcceptsMarketChild(t *testing.T) {
	for _, triggerType := range []string{"stop_loss", "take_profit"} {
		req, err := createConditional(triggerType, "buy", "market", nil, CreateTriggerOptions{})
		if err != nil {
			t.Fatalf("%s buy market: %v", triggerType, err)
		}
		if conditionalOf(req).GetChild().GetMarketIoc() == nil {
			t.Fatalf("%s buy: market child missing", triggerType)
		}
	}
}

func TestTrailingStopSlippageCapValidation(t *testing.T) {
	distance := int32(100)
	ticks := int64(5)
	bps := int32(10)
	create := func(opts CreateTriggerOptions) (*triggersv1.CreateTriggerRequest, error) {
		opts.TrailingDistanceBps = &distance
		return CreateTriggerToProto(1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
			"market", nil, "", "", nil, nil, false, 8, opts)
	}
	_, err := create(CreateTriggerOptions{MaxSlippageTicks: &ticks, MaxSlippageBps: &bps})
	wantValidation(t, err, "at most one of max_slippage_ticks or max_slippage_bps")
	full := int32(10_000)
	_, err = create(CreateTriggerOptions{MaxSlippageBps: &full})
	wantValidation(t, err, "between 1 and 9999")
	req, err := create(CreateTriggerOptions{MaxSlippageBps: &bps})
	if err != nil {
		t.Fatal(err)
	}
	if req.GetTrigger().GetTrailingStop().GetMaxSlippageBps() != bps {
		t.Fatalf("trailing cap %+v", req.GetTrigger().GetTrailingStop())
	}
}

func TestModifyTriggerSlippageCap(t *testing.T) {
	ticks := int64(5)
	bps := int32(10)
	_, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageTicks: &ticks, MaxSlippageBps: &bps})
	wantValidation(t, err, "at most one of max_slippage_ticks or max_slippage_bps")

	negative := int64(-1)
	_, err = ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageTicks: &negative})
	wantValidation(t, err, "must not be negative")

	req, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageTicks: &ticks})
	if err != nil {
		t.Fatal(err)
	}
	if req.GetMaxSlippageTicks() != ticks || req.LimitPriceTicks != nil {
		t.Fatalf("modify %+v", req)
	}
}

// Tick caps are int64 end to end: a value beyond the int32 range reaches the
// wire unchanged on create and modify.
func TestSlippageTicksKeepInt64Range(t *testing.T) {
	large := int64(math.MaxInt64)
	req, err := createConditional("stop_loss", "sell", "market", nil, CreateTriggerOptions{MaxSlippageTicks: &large})
	if err != nil {
		t.Fatal(err)
	}
	if got := conditionalOf(req).GetChild().GetMarketIoc().GetMaxSlippageTicks(); got != large {
		t.Fatalf("create max_slippage_ticks=%d", got)
	}
	distance := int64(math.MaxInt32) + 1
	trailing, err := CreateTriggerToProto(1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.1"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceTicks: &distance, MaxSlippageTicks: &large})
	if err != nil {
		t.Fatal(err)
	}
	if ts := trailing.GetTrigger().GetTrailingStop(); ts.GetTrailingDistanceTicks() != distance || ts.GetMaxSlippageTicks() != large {
		t.Fatalf("trailing %+v", ts)
	}
	mod, err := ModifyTriggerToProto("1", 7, nil, ModifyTriggerOptions{MaxSlippageTicks: &large})
	if err != nil {
		t.Fatal(err)
	}
	if mod.GetMaxSlippageTicks() != large {
		t.Fatalf("modify max_slippage_ticks=%d", mod.GetMaxSlippageTicks())
	}
}
