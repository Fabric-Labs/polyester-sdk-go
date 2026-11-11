package codecs

import (
	"testing"

	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// createTrigger builds one create request of each strategy with tif and opts.
func createTrigger(kind, orderType, tif string, opts CreateTriggerOptions) (*triggersv1.CreateTriggerRequest, error) {
	var trigger, limit *models.PriceInput
	triggerPrice := models.PriceFromDecimal("60000")
	limitPrice := models.PriceFromDecimal("59950")
	switch kind {
	case "stop_loss", "take_profit":
		trigger = &triggerPrice
		if orderType == "limit" {
			limit = &limitPrice
		}
	case "trailing_stop":
		distance := int32(100)
		opts.TrailingDistanceBps = &distance
	case "twap":
		duration, slice := int64(60_000), int64(10_000)
		opts.TwapDurationMs, opts.TwapSliceIntervalMs = &duration, &slice
		if orderType == "limit" {
			limit = &limitPrice
		}
	case "ladder":
		low, high := models.PriceFromDecimal("58000"), models.PriceFromDecimal("59000")
		levels := int32(4)
		opts.LadderPriceMin, opts.LadderPriceMax, opts.LadderLevels = &low, &high, &levels
	}
	return CreateTriggerToProto(
		1, "BTC-USDT", kind, trigger, "sell", models.QtyFromDecimal("0.01"),
		orderType, limit, "", tif, nil, nil, false, 8, opts,
	)
}

// POLY-8758: no trigger child has a GTD execution, so gtd is refused for every
// strategy rather than sent as an IOC or GTC child.
func TestPoly8758GTDIsRefusedForEveryTriggerStrategy(t *testing.T) {
	for _, tc := range []struct{ kind, orderType string }{
		{"stop_loss", "market"}, {"stop_loss", "limit"}, {"twap", "market"},
		{"twap", "limit"}, {"trailing_stop", "market"}, {"ladder", ""},
	} {
		if _, err := createTrigger(tc.kind, tc.orderType, "", CreateTriggerOptions{}); err != nil {
			t.Fatalf("%s %s without tif: %v", tc.kind, tc.orderType, err)
		}
		_, err := createTrigger(tc.kind, tc.orderType, "gtd", CreateTriggerOptions{})
		wantValidation(t, err, "tif must be")
	}
}

// POLY-8748: a TWAP market child's tick cap is positive, as every other
// create-time tick cap is.
func TestPoly8748TwapMarketRefusesNonpositiveSlippageTicks(t *testing.T) {
	for _, ticks := range []int64{-5, 0} {
		ticks := ticks
		_, err := createTrigger("twap", "market", "", CreateTriggerOptions{MaxSlippageTicks: &ticks})
		wantValidation(t, err, "max_slippage_ticks")
	}
	one := int64(1)
	req, err := createTrigger("twap", "market", "", CreateTriggerOptions{MaxSlippageTicks: &one})
	if err != nil {
		t.Fatal(err)
	}
	if got := req.GetTrigger().GetTwap().GetMarketIoc().GetMaxSlippageTicks(); got != 1 {
		t.Fatalf("max_slippage_ticks=%d, want 1", got)
	}
	// Controls: no cap sends none, and two caps are still refused.
	req, err = createTrigger("twap", "market", "", CreateTriggerOptions{})
	if err != nil || req.GetTrigger().GetTwap().GetMarketIoc().GetMaxSlippage() != nil {
		t.Fatalf("no cap: req=%v err=%v", req, err)
	}
	bps := int32(1)
	_, err = createTrigger("twap", "market", "", CreateTriggerOptions{MaxSlippageTicks: &one, MaxSlippageBps: &bps})
	wantValidation(t, err, "at most one")
}
