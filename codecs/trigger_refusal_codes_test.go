package codecs

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func refI64(v int64) *int64   { return &v }
func refI32(v int32) *int32   { return &v }
func refStr(v string) *string { return &v }
func refPrice(v string) *models.PriceInput {
	p := models.PriceFromDecimal(v)
	return &p
}

// triggerCase describes one CreateTriggerToProto call: a valid request of its
// strategy, changed by edit.
type triggerCase struct {
	kind, side, orderType string
	triggerPrice          *models.PriceInput
	limitPrice            *models.PriceInput
	qty                   models.QtyInput
	clientTriggerID       *string
	opts                  CreateTriggerOptions
}

func createCodedTrigger(c triggerCase) error {
	qty := c.qty
	if !qty.IsSet() {
		qty = models.QtyFromDecimal("0.001")
	}
	_, err := CreateTriggerToProto(1, "BTC-USDT", c.kind, c.triggerPrice, c.side, qty, c.orderType, c.limitPrice, "", "", nil, c.clientTriggerID, false, 8, c.opts)
	return err
}

func requireCode(t *testing.T, name string, err error, want orderv1.ErrorCode) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("%s: err=%v, want a ValidationError", name, err)
	}
	if validation.Code != want.String() {
		t.Fatalf("%s: code %q, want %s (%v)", name, validation.Code, want, err)
	}
}

// A trigger the SDK refuses before sending carries the code the API answers
// for the same field, so a pre-checked request and a server refusal read
// alike.
func TestTriggerRefusalsCarryTheAPIsFieldCodes(t *testing.T) {
	stop := func(edit func(*triggerCase)) triggerCase {
		c := triggerCase{kind: "stop_loss", side: "sell", orderType: "market", triggerPrice: refPrice("60000")}
		edit(&c)
		return c
	}
	trailing := func(edit func(*triggerCase)) triggerCase {
		c := triggerCase{kind: "trailing_stop", side: "sell", opts: CreateTriggerOptions{TrailingDistanceBps: refI32(100)}}
		edit(&c)
		return c
	}
	twap := func(edit func(*triggerCase)) triggerCase {
		c := triggerCase{kind: "twap", side: "buy", orderType: "market", opts: CreateTriggerOptions{TwapDurationMs: refI64(60_000), TwapSliceIntervalMs: refI64(1_000)}}
		edit(&c)
		return c
	}
	ladder := func(edit func(*triggerCase)) triggerCase {
		c := triggerCase{kind: "ladder", side: "buy", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMin: refPrice("30000"), LadderPriceMax: refPrice("31000"), LadderLevels: refI32(5)}}
		edit(&c)
		return c
	}
	for _, tc := range []struct {
		name string
		c    triggerCase
		want orderv1.ErrorCode
	}{
		{"client trigger id too long", stop(func(c *triggerCase) { c.clientTriggerID = refStr(strings.Repeat("a", 37)) }), orderv1.ErrorCode_ERROR_CODE_CLIENT_TRIGGER_ID_INVALID},
		{"qty zero", stop(func(c *triggerCase) { c.qty = models.QtyFromScaledInt(0) }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_QTY_INVALID},
		{"qty malformed", stop(func(c *triggerCase) { c.qty = models.QtyFromDecimal("-1") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_QTY_INVALID},
		{"trigger price malformed", stop(func(c *triggerCase) { c.triggerPrice = refPrice("abc") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"limit child without price", stop(func(c *triggerCase) { c.orderType = "limit" }), orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
		{"limit child price malformed", stop(func(c *triggerCase) { c.orderType, c.limitPrice = "limit", refPrice("abc") }), orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
		{"slippage ticks zero", stop(func(c *triggerCase) { c.opts.MaxSlippageTicks = refI64(0) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID},
		{"slippage bps zero", stop(func(c *triggerCase) { c.opts.MaxSlippageBps = refI32(0) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID},
		{"slippage bps above 10000", stop(func(c *triggerCase) { c.opts.MaxSlippageBps = refI32(10_001) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID},
		{"slippage bps 10000 on a sell", stop(func(c *triggerCase) { c.opts.MaxSlippageBps = refI32(10_000) }), orderv1.ErrorCode_ERROR_CODE_MAX_SLIPPAGE_INVALID},
		{"trailing without distance", trailing(func(c *triggerCase) { c.opts.TrailingDistanceBps = nil }), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"trailing distance ticks zero", trailing(func(c *triggerCase) {
			c.opts.TrailingDistanceBps, c.opts.TrailingDistanceTicks = nil, refI64(0)
		}), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"trailing distance bps above 10000", trailing(func(c *triggerCase) { c.opts.TrailingDistanceBps = refI32(10_001) }), orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"trailing limit price zero", trailing(func(c *triggerCase) { c.side, c.limitPrice = "buy", refPrice("0") }), orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
		{"trailing activation price malformed", trailing(func(c *triggerCase) { c.opts.ActivationPrice = refPrice("-1") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_INVALID},
		{"trailing slippage bps 10000", trailing(func(c *triggerCase) { c.opts.MaxSlippageBps = refI32(10_000) }), orderv1.ErrorCode_ERROR_CODE_MAX_SLIPPAGE_INVALID},
		{"trailing slippage ticks negative", trailing(func(c *triggerCase) { c.opts.MaxSlippageTicks = refI64(-1) }), orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID},
		{"twap duration short", twap(func(c *triggerCase) { c.opts.TwapDurationMs = refI64(999) }), orderv1.ErrorCode_ERROR_CODE_TWAP_DURATION_INVALID},
		{"twap without duration", twap(func(c *triggerCase) { c.opts.TwapDurationMs = nil }), orderv1.ErrorCode_ERROR_CODE_TWAP_DURATION_INVALID},
		{"twap interval short", twap(func(c *triggerCase) { c.opts.TwapSliceIntervalMs = refI64(99) }), orderv1.ErrorCode_ERROR_CODE_TWAP_INTERVAL_INVALID},
		{"twap interval above duration", twap(func(c *triggerCase) { c.opts.TwapSliceIntervalMs = refI64(60_001) }), orderv1.ErrorCode_ERROR_CODE_TWAP_INTERVAL_INVALID},
		{"twap limit slice without price", twap(func(c *triggerCase) { c.orderType = "limit" }), orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
		{"ladder levels 1", ladder(func(c *triggerCase) { c.opts.LadderLevels = refI32(1) }), orderv1.ErrorCode_ERROR_CODE_LADDER_LEVELS_INVALID},
		{"ladder levels 101", ladder(func(c *triggerCase) { c.opts.LadderLevels = refI32(101) }), orderv1.ErrorCode_ERROR_CODE_LADDER_LEVELS_INVALID},
		{"ladder max not above min", ladder(func(c *triggerCase) { c.opts.LadderPriceMax = refPrice("30000") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"ladder min malformed", ladder(func(c *triggerCase) { c.opts.LadderPriceMin = refPrice("abc") }), orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
	} {
		requireCode(t, tc.name, createCodedTrigger(tc.c), tc.want)
	}
}

// ModifyTrigger answers the same codes; its caps may be 0, which clears them.
func TestModifyTriggerRefusalsCarryTheAPIsFieldCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts ModifyTriggerOptions
		want orderv1.ErrorCode
	}{
		{"trigger price malformed", ModifyTriggerOptions{TriggerPrice: refPrice("abc")}, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"activation price malformed", ModifyTriggerOptions{ActivationPrice: refPrice("-1")}, orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_INVALID},
		{"trailing distance bps zero", ModifyTriggerOptions{TrailingDistanceBps: refI32(0)}, orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID},
		{"slippage ticks negative", ModifyTriggerOptions{MaxSlippageTicks: refI64(-1)}, orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID},
		{"slippage bps above 10000", ModifyTriggerOptions{MaxSlippageBps: refI32(10_001)}, orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID},
		{"slippage bps negative", ModifyTriggerOptions{MaxSlippageBps: refI32(-1)}, orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID},
	} {
		_, err := ModifyTriggerToProto("2", 1, "BTC-USDT", nil, tc.opts)
		requireCode(t, tc.name, err, tc.want)
	}
	for _, opts := range []ModifyTriggerOptions{{MaxSlippageTicks: refI64(0)}, {MaxSlippageBps: refI32(0)}} {
		if _, err := ModifyTriggerToProto("2", 1, "BTC-USDT", nil, opts); err != nil {
			t.Fatalf("a zero cap clears it: %v", err)
		}
	}
}

// CreateOco answers BAD_QTY for its quantity, TRIGGER_PRICE_INVALID for a
// leg's trigger price and BAD_PRICE for a leg's limit price.
func TestOcoRefusalsCarryTheAPIsFieldCodes(t *testing.T) {
	valid := func() models.CreateOcoInput {
		limit := models.PriceFromDecimal("71000")
		return models.CreateOcoInput{
			Symbol: "BTC-USDT", Side: "sell", Qty: models.QtyFromDecimal("0.001"),
			TakeProfit: models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("70000"), LimitPrice: &limit},
			StopLoss:   models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("60000")},
		}
	}
	if _, err := CreateOcoToProto(1, valid(), nil, 8); err != nil {
		t.Fatalf("valid OCO: %v", err)
	}
	zeroLimit := models.PriceFromDecimal("0")
	for _, tc := range []struct {
		name string
		edit func(*models.CreateOcoInput)
		want orderv1.ErrorCode
	}{
		{"qty zero", func(in *models.CreateOcoInput) { in.Qty = models.QtyFromScaledInt(0) }, orderv1.ErrorCode_ERROR_CODE_BAD_QTY},
		{"stop-loss trigger price zero", func(in *models.CreateOcoInput) { in.StopLoss.TriggerPrice = models.PriceFromDecimal("0") }, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"take-profit trigger price missing", func(in *models.CreateOcoInput) { in.TakeProfit.TriggerPrice = models.PriceInput{} }, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID},
		{"take-profit limit price zero", func(in *models.CreateOcoInput) { in.TakeProfit.LimitPrice = &zeroLimit }, orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
	} {
		in := valid()
		tc.edit(&in)
		_, err := CreateOcoToProto(1, in, nil, 8)
		requireCode(t, tc.name, err, tc.want)
	}
}

// The fee-asset rule for every trigger strategy: a sell pays its fee in quote,
// refused locally with the API's ERROR_CODE_FEE_ASSET_NOT_ALLOWED. A buy may
// ask for base; whether its pair allows it is the server's to say.
func TestTriggerFeeAssetBaseOnASellIsRefused(t *testing.T) {
	cases := map[string]triggerCase{
		"stop_loss":     {kind: "stop_loss", orderType: "market", triggerPrice: refPrice("60000")},
		"take_profit":   {kind: "take_profit", orderType: "market", triggerPrice: refPrice("70000")},
		"trailing_stop": {kind: "trailing_stop", opts: CreateTriggerOptions{TrailingDistanceBps: refI32(100)}},
		"twap":          {kind: "twap", orderType: "market", opts: CreateTriggerOptions{TwapDurationMs: refI64(60_000), TwapSliceIntervalMs: refI64(1_000)}},
		"ladder":        {kind: "ladder", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMin: refPrice("30000"), LadderPriceMax: refPrice("31000"), LadderLevels: refI32(5)}},
	}
	for kind, c := range cases {
		for _, asset := range []string{"base", "BASE", "received"} {
			sell := c
			sell.side = "sell"
			sell.opts.FeeAsset = refStr(asset)
			requireCode(t, kind+" sell "+asset, createCodedTrigger(sell), orderv1.ErrorCode_ERROR_CODE_FEE_ASSET_NOT_ALLOWED)
		}
		for _, asset := range []string{"quote", "base"} {
			ok := c
			ok.side = "buy"
			if kind == "trailing_stop" {
				ok.limitPrice = refPrice("70000")
			}
			ok.opts.FeeAsset = refStr(asset)
			if err := createCodedTrigger(ok); err != nil {
				t.Fatalf("%s buy fee_asset=%s: %v", kind, asset, err)
			}
		}
		quote := c
		quote.side = "sell"
		quote.opts.FeeAsset = refStr("quote")
		if err := createCodedTrigger(quote); err != nil {
			t.Fatalf("%s sell fee_asset=quote: %v", kind, err)
		}
	}
}

// The order path's refusal carries the same code.
func TestOrderFeeAssetBaseOnASellCarriesTheAPIsCode(t *testing.T) {
	symbolID := uint32(1)
	_, err := CreateOrderToProto(models.CreateOrderRequest{
		Symbol: refStr("BTC-USDT"), SymbolID: &symbolID, Side: "sell", OrderType: "limit", TIF: refStr("gtc"),
		Qty: models.QtyFromDecimal("0.001"), Price: refPrice("60000"), FeeAsset: refStr("base"),
	}, 8, 6)
	requireCode(t, "sell order fee_asset=base", err, orderv1.ErrorCode_ERROR_CODE_FEE_ASSET_NOT_ALLOWED)
}

// A client trigger ID is what the contract says, 1 to 36 bytes of UTF-8 in
// any characters, sent unchanged; only a longer or non-UTF-8 ID is refused.
func TestClientTriggerIDFollowsTheContract(t *testing.T) {
	if clientTriggerIDMaxBytes != 36 {
		t.Fatalf("max bytes = %d, want the contract's 36", clientTriggerIDMaxBytes)
	}
	for _, id := range []string{"ladder-btc-1", "stop loss #1", " padded ", "ordre-été", strings.Repeat("é", 18), strings.Repeat("a", 36)} {
		req, err := CreateTriggerToProto(1, "BTC-USDT", "stop_loss", refPrice("60000"), "sell", models.QtyFromDecimal("0.001"), "market", nil, "", "", nil, refStr(id), false, 8, CreateTriggerOptions{})
		if err != nil {
			t.Fatalf("%q: %v", id, err)
		}
		if got := req.GetTrigger().GetClientTriggerId(); got != id {
			t.Fatalf("%q sent as %q", id, got)
		}
	}
	// An explicit empty ID is refused, never replaced: nil asks for one.
	for _, id := range []string{"", strings.Repeat("a", 37), strings.Repeat("é", 19), "bad\xff"} {
		_, err := CreateTriggerToProto(1, "BTC-USDT", "stop_loss", refPrice("60000"), "sell", models.QtyFromDecimal("0.001"), "market", nil, "", "", nil, refStr(id), false, 8, CreateTriggerOptions{})
		requireCode(t, id, err, orderv1.ErrorCode_ERROR_CODE_CLIENT_TRIGGER_ID_INVALID)
	}
}

// A nil client trigger ID gets a generated one; each create its own.
func TestNilClientTriggerIDIsGenerated(t *testing.T) {
	ids := map[string]bool{}
	for range 2 {
		req, err := CreateTriggerToProto(1, "BTC-USDT", "stop_loss", refPrice("60000"), "sell", models.QtyFromDecimal("0.001"), "market", nil, "", "", nil, nil, false, 8, CreateTriggerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		id := req.GetTrigger().GetClientTriggerId()
		if id == "" || ids[id] {
			t.Fatalf("generated ID %q (seen %v)", id, ids)
		}
		ids[id] = true
	}
}
