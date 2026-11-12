package codecs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var triggerTypeToProto = map[string]triggersv1.TriggerType{
	"stop_loss":     triggersv1.TriggerType_STOP_LOSS,
	"take_profit":   triggersv1.TriggerType_TAKE_PROFIT,
	"trailing_stop": triggersv1.TriggerType_TRAILING_STOP,
	"twap":          triggersv1.TriggerType_TWAP,
	"ladder":        triggersv1.TriggerType_LADDER,
}

var triggerFeeAssetToProto = map[string]orderv1.FeeAsset{
	"quote":    orderv1.FeeAsset_QUOTE,
	"base":     orderv1.FeeAsset_BASE,
	"received": orderv1.FeeAsset_BASE, // legacy synonym for a received base fee
}

var selfTradePreventionModeToProto = map[string]orderv1.SelfTradePreventionMode{
	"expire_taker": orderv1.SelfTradePreventionMode_EXPIRE_TAKER,
	"expire_maker": orderv1.SelfTradePreventionMode_EXPIRE_MAKER,
	"expire_both":  orderv1.SelfTradePreventionMode_EXPIRE_BOTH,
}

// CreateTriggerOptions carries optional create-trigger fields beyond the core order params.
type CreateTriggerOptions struct {
	FeeAsset                *string
	SelfTradePreventionMode *string
	// TrailingDistanceTicks is the trailing distance as a price delta in
	// 1e-9 quote-unit ticks, the scale of every price tick: on BTC-USDT,
	// 50 USDT is 50_000_000_000 and 0.5 USDT is 500_000_000. Convert a
	// decimal distance with ParsePriceTicks("50", "trailing distance").
	TrailingDistanceTicks *int64
	// TrailingDistanceBps is the trailing distance in basis points of the
	// price (1 bp = 0.01%), from 1 to 10000.
	TrailingDistanceBps *int32
	ActivationPrice     *models.PriceInput
	MaxSlippageTicks    *int64
	MaxSlippageBps      *int32
	TwapDurationMs      *int64
	TwapSliceIntervalMs *int64
	LadderPriceMin      *models.PriceInput
	LadderPriceMax      *models.PriceInput
	LadderLevels        *int32
	LadderDistribution  *string
}

// TriggerTypeFromLabel parses a trigger type label (stop_loss, take_profit,
// trailing_stop, twap or ladder; case and "-" for "_" are accepted) into its
// contract value.
func TriggerTypeFromLabel(label string) (triggersv1.TriggerType, error) {
	key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(label), "-", "_"))
	if value, ok := triggerTypeToProto[key]; ok {
		return value, nil
	}
	return triggersv1.TriggerType_TRIGGER_TYPE_UNSPECIFIED, &errors.ValidationError{Msg: fmt.Sprintf("trigger_type %q must be stop_loss, take_profit, trailing_stop, twap, or ladder", label)}
}

func CreateTriggerToProto(symbolID uint32, symbol, triggerType string, triggerPrice *models.PriceInput, side string, qty models.QtyInput, orderType string, limitPrice *models.PriceInput, triggerPriceSource, tif string, subAccountID *string, clientTriggerID *string, postOnly bool, quantityScale int, opts CreateTriggerOptions) (*triggersv1.CreateTriggerRequest, error) {
	if symbolID == 0 {
		return nil, &errors.ValidationError{Msg: "triggers.create requires a resolved non-zero symbol_id"}
	}
	typeKey := strings.ToLower(strings.ReplaceAll(triggerType, "-", "_"))
	if _, ok := triggerTypeToProto[typeKey]; !ok {
		return nil, &errors.ValidationError{Msg: "trigger_type must be stop_loss, take_profit, trailing_stop, twap, or ladder"}
	}
	if err := validateTriggerOptionApplicability(typeKey, triggerPrice, side, orderType, limitPrice, triggerPriceSource, tif, postOnly, opts); err != nil {
		return nil, err
	}
	qtyScaled, err := ResolveQtyScaled(qty, quantityScale, "qty", symbol, &symbolID)
	if err != nil {
		return nil, err
	}
	intent := &triggersv1.TriggerIntent{
		SymbolId:  symbolID,
		QtyScaled: qtyScaled,
	}
	if clientTriggerID != nil && *clientTriggerID != "" {
		validated, err := requiredClientID(*clientTriggerID, "client_trigger_id")
		if err != nil {
			return nil, err
		}
		intent.ClientTriggerId = validated
	} else {
		intent.ClientTriggerId = newTriggerRequestID()
	}
	if opts.FeeAsset != nil {
		asset, ok := triggerFeeAssetToProto[strings.ToLower(*opts.FeeAsset)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "fee_asset must be quote or base"}
		}
		intent.FeeAsset = asset
	}
	if opts.SelfTradePreventionMode != nil {
		mode, ok := selfTradePreventionModeToProto[strings.ToLower(*opts.SelfTradePreventionMode)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "self_trade_prevention_mode must be expire_taker, expire_maker, or expire_both"}
		}
		intent.SelfTradePreventionMode = mode
	}

	switch typeKey {
	case "stop_loss", "take_profit":
		sideEnum, ok := orderSideToProto[strings.ToLower(side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		triggerTicks := int64(0)
		if triggerPrice != nil && triggerPrice.IsSet() {
			parsed, err := ResolvePriceTicks(*triggerPrice, "trigger_price", symbol)
			if err != nil {
				return nil, err
			}
			triggerTicks = parsed
		}
		child, err := conditionalChildToProto(orderType, tif, limitPrice, symbol, postOnly, sideEnum, opts.MaxSlippageTicks, opts.MaxSlippageBps)
		if err != nil {
			return nil, err
		}
		source, err := triggerPriceSourceToProto(triggerPriceSource, "trigger_price_source")
		if err != nil {
			return nil, err
		}
		cond := &triggersv1.ConditionalTrigger{
			TriggerPriceTicks:  triggerTicks,
			Side:               sideEnum,
			Child:              child,
			TriggerPriceSource: source,
		}
		if typeKey == "stop_loss" {
			intent.Strategy = &triggersv1.TriggerIntent_StopLoss{StopLoss: cond}
		} else {
			intent.Strategy = &triggersv1.TriggerIntent_TakeProfit{TakeProfit: cond}
		}
	case "trailing_stop":
		if opts.TrailingDistanceTicks != nil && opts.TrailingDistanceBps != nil {
			return nil, &errors.ValidationError{Msg: "set at most one of trailing_distance_ticks or trailing_distance_bps"}
		}
		// A sell trails the peak and fires on a fall; a buy trails the trough
		// and fires on a rise, and limit_price is its maximum price: the most
		// its child pays per unit, which also sizes the reserved funds.
		sideEnum, ok := orderSideToProto[strings.ToLower(side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		source, err := triggerPriceSourceToProto(triggerPriceSource, "trigger_price_source")
		if err != nil {
			return nil, err
		}
		trailing := &triggersv1.TrailingStopTrigger{Side: sideEnum, TriggerPriceSource: source}
		hasLimitPrice := limitPrice != nil && limitPrice.IsSet()
		switch {
		case sideEnum == orderv1.Side_BUY && !hasLimitPrice:
			return nil, &errors.ValidationError{Msg: "trailing_stop with side=buy requires limit_price, the maximum price the child pays"}
		case sideEnum == orderv1.Side_SELL && hasLimitPrice:
			return nil, &errors.ValidationError{Msg: "trailing_stop with side=sell does not accept limit_price; only a buy has a maximum price"}
		case hasLimitPrice:
			ticks, err := ResolvePriceTicks(*limitPrice, "limit_price", symbol)
			if err != nil {
				return nil, err
			}
			if ticks <= 0 {
				return nil, &errors.ValidationError{Msg: "limit_price must be positive"}
			}
			trailing.LimitPriceTicks = ticks
		}
		switch {
		case opts.TrailingDistanceTicks != nil:
			trailing.TrailingDistance = &triggersv1.TrailingStopTrigger_TrailingDistanceTicks{TrailingDistanceTicks: *opts.TrailingDistanceTicks}
		case opts.TrailingDistanceBps != nil:
			if err := validateBps("trailing_distance_bps", *opts.TrailingDistanceBps, false); err != nil {
				return nil, err
			}
			trailing.TrailingDistance = &triggersv1.TrailingStopTrigger_TrailingDistanceBps{TrailingDistanceBps: *opts.TrailingDistanceBps}
		default:
			return nil, &errors.ValidationError{Msg: "trailing_stop requires trailing_distance_ticks or trailing_distance_bps"}
		}
		if opts.ActivationPrice != nil && opts.ActivationPrice.IsSet() {
			ticks, err := ResolvePriceTicks(*opts.ActivationPrice, "activation_price", symbol)
			if err != nil {
				return nil, err
			}
			trailing.ActivationPriceTicks = ticks
		}
		if trailing.LimitPriceTicks > 0 && trailing.ActivationPriceTicks > trailing.LimitPriceTicks {
			return nil, &errors.ValidationError{Msg: "trailing_stop limit_price must be at least activation_price"}
		}
		// A trailing stop takes no 100% cap on either side.
		ticks, bps, err := createSlippageCap(opts.MaxSlippageTicks, opts.MaxSlippageBps, MaxSellingSlippageBps)
		if err != nil {
			return nil, err
		}
		switch {
		case ticks != nil:
			trailing.MaxSlippage = &triggersv1.TrailingStopTrigger_MaxSlippageTicks{MaxSlippageTicks: *ticks}
		case bps != nil:
			trailing.MaxSlippage = &triggersv1.TrailingStopTrigger_MaxSlippageBps{MaxSlippageBps: *bps}
		}
		intent.Strategy = &triggersv1.TriggerIntent_TrailingStop{TrailingStop: trailing}
	case "twap":
		sideEnum, ok := orderSideToProto[strings.ToLower(side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		twap := &triggersv1.TwapTrigger{Side: sideEnum}
		if opts.TwapDurationMs != nil {
			twap.DurationMs = *opts.TwapDurationMs
		}
		if opts.TwapSliceIntervalMs != nil {
			twap.SliceIntervalMs = *opts.TwapSliceIntervalMs
		}
		switch strings.ToLower(orderType) {
		case "market":
			ioc := &triggersv1.TwapMarketIoc{}
			// A slice that sells takes no 100% cap: it would leave no
			// positive price to sell at.
			maxBps := MaxSlippageBpsLimit
			if sideEnum == orderv1.Side_SELL {
				maxBps = MaxSellingSlippageBps
			}
			ticks, bps, err := createSlippageCap(opts.MaxSlippageTicks, opts.MaxSlippageBps, maxBps)
			if err != nil {
				return nil, err
			}
			switch {
			case ticks != nil:
				ioc.MaxSlippage = &triggersv1.TwapMarketIoc_MaxSlippageTicks{MaxSlippageTicks: *ticks}
			case bps != nil:
				ioc.MaxSlippage = &triggersv1.TwapMarketIoc_MaxSlippageBps{MaxSlippageBps: *bps}
			}
			twap.Execution = &triggersv1.TwapTrigger_MarketIoc{MarketIoc: ioc}
		case "limit":
			if limitPrice == nil || !limitPrice.IsSet() {
				return nil, &errors.ValidationError{Msg: "twap limit slices require limit_price"}
			}
			ticks, err := ResolvePriceTicks(*limitPrice, "limit_price", symbol)
			if err != nil {
				return nil, err
			}
			twap.Execution = &triggersv1.TwapTrigger_LimitGtc{LimitGtc: &triggersv1.TwapLimitGtc{PriceTicks: ticks}}
		default:
			return nil, &errors.ValidationError{Msg: "order_type must be limit or market"}
		}
		intent.Strategy = &triggersv1.TriggerIntent_Twap{Twap: twap}
	case "ladder":
		sideEnum, ok := orderSideToProto[strings.ToLower(side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		if opts.LadderDistribution != nil {
			if dist := strings.ToLower(*opts.LadderDistribution); dist != "" && dist != "linear" {
				return nil, &errors.ValidationError{Msg: "ladder only supports linear distribution"}
			}
		}
		ladder := &triggersv1.LadderTrigger{Side: sideEnum, PostOnly: postOnly}
		var hasMin, hasMax bool
		if opts.LadderPriceMin != nil && opts.LadderPriceMin.IsSet() {
			ticks, err := ResolvePriceTicks(*opts.LadderPriceMin, "ladder_price_min", symbol)
			if err != nil {
				return nil, err
			}
			ladder.PriceMinTicks = ticks
			hasMin = true
		}
		if opts.LadderPriceMax != nil && opts.LadderPriceMax.IsSet() {
			ticks, err := ResolvePriceTicks(*opts.LadderPriceMax, "ladder_price_max", symbol)
			if err != nil {
				return nil, err
			}
			ladder.PriceMaxTicks = ticks
			hasMax = true
		}
		if hasMin && hasMax && ladder.PriceMinTicks >= ladder.PriceMaxTicks {
			return nil, &errors.ValidationError{Msg: "ladder_price_min must be strictly less than ladder_price_max"}
		}
		if opts.LadderLevels != nil {
			ladder.Levels = *opts.LadderLevels
		}
		intent.Strategy = &triggersv1.TriggerIntent_Ladder{Ladder: ladder}
	}

	req := &triggersv1.CreateTriggerRequest{Trigger: intent}
	if subAccountID != nil {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		req.SubaccountId = &sub
	}
	return req, nil
}

// validateTriggerOptionApplicability prevents flat options from disappearing
// when a different strategy or a fixed child execution is selected.
func validateTriggerOptionApplicability(kind string, triggerPrice *models.PriceInput, side, orderType string, limitPrice *models.PriceInput, source, tif string, postOnly bool, opts CreateTriggerOptions) error {
	for _, option := range []struct {
		present         bool
		name, supported string
	}{
		{opts.TrailingDistanceTicks != nil, "trailing_distance_ticks", "trailing_stop"},
		{opts.TrailingDistanceBps != nil, "trailing_distance_bps", "trailing_stop"},
		{opts.ActivationPrice != nil && opts.ActivationPrice.IsSet(), "activation_price", "trailing_stop"},
		{opts.TwapDurationMs != nil, "twap_duration_ms", "twap"},
		{opts.TwapSliceIntervalMs != nil, "twap_slice_interval_ms", "twap"},
		{opts.LadderPriceMin != nil && opts.LadderPriceMin.IsSet(), "ladder_price_min", "ladder"},
		{opts.LadderPriceMax != nil && opts.LadderPriceMax.IsSet(), "ladder_price_max", "ladder"},
		{opts.LadderLevels != nil, "ladder_levels", "ladder"},
		{opts.LadderDistribution != nil, "ladder_distribution", "ladder"},
	} {
		if option.present && kind != option.supported {
			return &errors.ValidationError{Msg: option.name + " applies only to " + option.supported}
		}
	}
	if triggerPrice != nil && triggerPrice.IsSet() && kind != "stop_loss" && kind != "take_profit" {
		return &errors.ValidationError{Msg: "trigger_price applies only to stop_loss or take_profit"}
	}
	if kind == "twap" || kind == "ladder" {
		parsed, err := triggerPriceSourceToProto(source, "trigger_price_source")
		if err != nil {
			return err
		}
		if parsed != orderv1.TriggerPriceSource_TRIGGER_PRICE_SOURCE_UNSPECIFIED {
			return &errors.ValidationError{Msg: "trigger_price_source applies only to stop_loss, take_profit or trailing_stop"}
		}
	}
	orderType = strings.ToLower(orderType)
	validOrderType := orderType == "market" || orderType == "limit"
	if orderType == "" && (kind == "trailing_stop" || kind == "ladder") {
		validOrderType = true
	}
	if !validOrderType {
		return &errors.ValidationError{Msg: "order_type must be limit or market"}
	}
	// A ladder places limit GTC orders only: a market order type is refused,
	// never sent as limit.
	if kind == "ladder" && orderType != "" && orderType != "limit" {
		return &errors.ValidationError{Msg: "order_type=" + orderType + " does not apply to a ladder: a ladder places limit GTC orders only"}
	}
	if postOnly && (kind == "trailing_stop" || kind == "twap") {
		return &errors.ValidationError{Msg: "post_only applies only to limit GTC stops or ladder"}
	}
	if limitPrice != nil && limitPrice.IsSet() && (kind == "ladder" || (kind != "trailing_stop" && orderType == "market")) {
		return &errors.ValidationError{Msg: "limit_price applies only to limit children or a buy trailing_stop"}
	}
	if (kind == "ladder" || (kind == "twap" && orderType == "limit")) && (opts.MaxSlippageTicks != nil || opts.MaxSlippageBps != nil) {
		return &errors.ValidationError{Msg: "max_slippage_ticks and max_slippage_bps apply only to market children or trailing_stop"}
	}
	tif = strings.ToLower(tif)
	if tif != "" && tif != "gtc" && tif != "ioc" && tif != "fok" {
		return &errors.ValidationError{Msg: "tif must be gtc, ioc or fok"}
	}
	// Empty leaves execution unspecified. A market child and a trailing stop
	// always execute IOC, so they take only ioc: an explicit gtc or fok is
	// refused, never sent as IOC.
	if kind == "ladder" || (kind == "twap" && orderType == "limit") {
		if tif != "" && tif != "gtc" {
			return &errors.ValidationError{Msg: "tif must be gtc for ladder or twap limit slices"}
		}
	} else if (kind == "trailing_stop" || orderType == "market") && tif != "" && tif != "ioc" {
		return &errors.ValidationError{Msg: "tif=" + tif + " applies only to limit children; a market child or trailing_stop executes IOC"}
	}
	if kind == "trailing_stop" && strings.EqualFold(side, "sell") && orderType == "limit" {
		return &errors.ValidationError{Msg: "order_type=limit applies only to buy trailing_stop children"}
	}
	return nil
}

// conditionalChildToProto maps flat (order_type, tif, limit_price, post_only)
// params onto a stop-loss / take-profit child execution variant. A market
// child carries the optional slippage cap; either side may use any child.
func conditionalChildToProto(orderType, tif string, limitPrice *models.PriceInput, symbol string, postOnly bool, side orderv1.Side, maxSlippageTicks *int64, maxSlippageBps *int32) (*triggersv1.ConditionalChildExecution, error) {
	switch strings.ToLower(orderType) {
	case "market":
		if postOnly {
			return nil, &errors.ValidationError{Msg: "post_only is only valid for limit GTC executions"}
		}
		maxBps := MaxSlippageBpsLimit
		if side == orderv1.Side_SELL {
			maxBps = MaxSellingSlippageBps
		}
		ticks, bps, err := createSlippageCap(maxSlippageTicks, maxSlippageBps, maxBps)
		if err != nil {
			return nil, err
		}
		ioc := &triggersv1.TriggerMarketIoc{}
		switch {
		case ticks != nil:
			ioc.MaxSlippage = &triggersv1.TriggerMarketIoc_MaxSlippageTicks{MaxSlippageTicks: *ticks}
		case bps != nil:
			ioc.MaxSlippage = &triggersv1.TriggerMarketIoc_MaxSlippageBps{MaxSlippageBps: *bps}
		}
		return &triggersv1.ConditionalChildExecution{
			Execution: &triggersv1.ConditionalChildExecution_MarketIoc{MarketIoc: ioc},
		}, nil
	case "limit":
		if maxSlippageTicks != nil || maxSlippageBps != nil {
			return nil, &errors.ValidationError{Msg: "max_slippage_ticks and max_slippage_bps apply only to market children; a limit child is bounded by limit_price"}
		}
		if limitPrice == nil || !limitPrice.IsSet() {
			return nil, &errors.ValidationError{Msg: "limit trigger requires limit_price"}
		}
		ticks, err := ResolvePriceTicks(*limitPrice, "limit_price", symbol)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(tif) {
		case "ioc":
			if postOnly {
				return nil, &errors.ValidationError{Msg: "post_only is only valid for limit GTC executions"}
			}
			return &triggersv1.ConditionalChildExecution{
				Execution: &triggersv1.ConditionalChildExecution_LimitIoc{LimitIoc: &triggersv1.TriggerLimitIoc{PriceTicks: ticks}},
			}, nil
		case "fok":
			if postOnly {
				return nil, &errors.ValidationError{Msg: "post_only is only valid for limit GTC executions"}
			}
			return &triggersv1.ConditionalChildExecution{
				Execution: &triggersv1.ConditionalChildExecution_LimitFok{LimitFok: &triggersv1.TriggerLimitFok{PriceTicks: ticks}},
			}, nil
		default: // gtc or unspecified
			return &triggersv1.ConditionalChildExecution{
				Execution: &triggersv1.ConditionalChildExecution_LimitGtc{LimitGtc: &triggersv1.TriggerLimitGtc{PriceTicks: ticks, PostOnly: postOnly}},
			}, nil
		}
	default:
		return nil, &errors.ValidationError{Msg: "order_type must be limit or market"}
	}
}

// MaxSlippageBpsLimit is the product-wide trigger/order slippage BPS cap.
const MaxSlippageBpsLimit int32 = 10_000

// createSlippageCap validates an optional create-time slippage cap: at most
// one of ticks or bps, ticks positive, bps from 1 to maxBps.
func createSlippageCap(ticks *int64, bps *int32, maxBps int32) (*int64, *int32, error) {
	switch {
	case ticks != nil && bps != nil:
		return nil, nil, &errors.ValidationError{Msg: "set at most one of max_slippage_ticks or max_slippage_bps"}
	case ticks != nil && *ticks <= 0:
		return nil, nil, &errors.ValidationError{Msg: "max_slippage_ticks must be positive"}
	case bps != nil && (*bps < 1 || *bps > maxBps):
		return nil, nil, &errors.ValidationError{Msg: fmt.Sprintf("max_slippage_bps must be between 1 and %d", maxBps)}
	}
	return ticks, bps, nil
}

func validateBps(field string, bps int32, allowClear bool) error {
	return validateBpsAtMost(field, bps, allowClear, MaxSlippageBpsLimit)
}

func validateBpsAtMost(field string, bps int32, allowClear bool, maxBps int32) error {
	if allowClear {
		if bps < 0 || bps > maxBps {
			return &errors.ValidationError{Msg: fmt.Sprintf("%s must be between 0 and %d; 0 clears the cap", field, maxBps)}
		}
		return nil
	}
	if bps < 1 || bps > maxBps {
		return &errors.ValidationError{Msg: fmt.Sprintf("%s must be between 1 and %d", field, maxBps)}
	}
	return nil
}

// MaxSellingSlippageBps caps the bps slippage of anything that sells and of
// every trailing stop: a 100% cap would leave a sell no positive price.
const MaxSellingSlippageBps = MaxSlippageBpsLimit - 1

// ModifyTriggerOptions carries optional trigger patch fields. A child's
// limit price is fixed at create: cancel and create a new trigger to change
// it. MaxSlippage* applies to trailing stops and to stop-loss or take-profit
// triggers with a market child.
//
// Omitted ActivationPrice / MaxSlippage* preserve the current values.
// Send an explicit zero (PriceFromTicksInt(0) or *int32(0)) to clear an
// existing activation price or maximum-slippage cap.
type ModifyTriggerOptions struct {
	TriggerPrice *models.PriceInput
	// TrailingDistanceTicks is the new trailing distance as a price delta in
	// 1e-9 quote-unit ticks, as on CreateTriggerOptions.
	TrailingDistanceTicks *int64
	TrailingDistanceBps   *int32
	ActivationPrice       *models.PriceInput
	MaxSlippageTicks      *int64
	MaxSlippageBps        *int32
}

// ModifyTriggerToProto encodes a trigger patch. A zero symbolID targets the
// trigger's own market. symbol is that market's name when known: a price bound
// to another market is refused. Pass "" when the market is not known.
func ModifyTriggerToProto(triggerID string, symbolID uint32, symbol string, subAccountID *string, opts ModifyTriggerOptions) (*triggersv1.ModifyTriggerRequest, error) {
	if (opts.TriggerPrice == nil || !opts.TriggerPrice.IsSet()) &&
		opts.TrailingDistanceTicks == nil && opts.TrailingDistanceBps == nil &&
		(opts.ActivationPrice == nil || !opts.ActivationPrice.IsSet()) &&
		opts.MaxSlippageTicks == nil && opts.MaxSlippageBps == nil {
		return nil, &errors.ValidationError{Msg: "modify requires at least one of trigger_price, trailing_distance_ticks, trailing_distance_bps, activation_price, max_slippage_ticks, or max_slippage_bps"}
	}
	if opts.MaxSlippageTicks != nil && opts.MaxSlippageBps != nil {
		return nil, &errors.ValidationError{Msg: "set at most one of max_slippage_ticks or max_slippage_bps"}
	}
	if opts.TrailingDistanceTicks != nil && opts.TrailingDistanceBps != nil {
		return nil, &errors.ValidationError{Msg: "set at most one of trailing_distance_ticks or trailing_distance_bps"}
	}
	id, err := IDToInt(triggerID, "trigger_id")
	if err != nil {
		return nil, err
	}
	req := &triggersv1.ModifyTriggerRequest{TriggerId: id, SymbolId: symbolID}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		req.SubaccountId = &sub
	}
	if opts.TriggerPrice != nil && opts.TriggerPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*opts.TriggerPrice, "trigger_price", symbol)
		if err != nil {
			return nil, err
		}
		req.TriggerPriceTicks = &ticks
	}
	if opts.TrailingDistanceTicks != nil {
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceTicks{TrailingDistanceTicks: *opts.TrailingDistanceTicks}
	}
	if opts.TrailingDistanceBps != nil {
		if err := validateBps("trailing_distance_bps", *opts.TrailingDistanceBps, false); err != nil {
			return nil, err
		}
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceBps{TrailingDistanceBps: *opts.TrailingDistanceBps}
	}
	if opts.ActivationPrice != nil && opts.ActivationPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*opts.ActivationPrice, "activation_price", symbol)
		if err != nil {
			return nil, err
		}
		req.ActivationPriceTicks = &ticks
	}
	if opts.MaxSlippageTicks != nil {
		if *opts.MaxSlippageTicks < 0 {
			return nil, &errors.ValidationError{Msg: "max_slippage_ticks must not be negative; send 0 to clear the cap"}
		}
		req.MaxSlippage = &triggersv1.ModifyTriggerRequest_MaxSlippageTicks{MaxSlippageTicks: *opts.MaxSlippageTicks}
	}
	if opts.MaxSlippageBps != nil {
		// 0 to 10000, as ModifyTriggerRequest takes it: the patch does not
		// name the trigger's side or type, so the server decides 10000 (a
		// buy stop-loss or take-profit market child takes it; a sell or a
		// trailing stop answers MAX_SLIPPAGE_INVALID).
		if err := validateBps("max_slippage_bps", *opts.MaxSlippageBps, true); err != nil {
			return nil, err
		}
		req.MaxSlippage = &triggersv1.ModifyTriggerRequest_MaxSlippageBps{MaxSlippageBps: *opts.MaxSlippageBps}
	}
	return req, nil
}

func ParseRequiredUint64Decimal(raw, field string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !isDecimal(raw) {
		return 0, &errors.ValidationError{Msg: field + " must be a uint64 decimal string"}
	}
	v, err := IDToInt(raw, field)
	return v, err
}

// MaxBatchCancelTriggers is the contract's limit of trigger IDs per batch
// trigger cancel, 50. A larger batch is refused before sending.
var MaxBatchCancelTriggers = contractMaxRepeated(&triggersv1.BatchCancelTriggersRequest{}, "trigger_ids")

// CreateOcoToProto encodes an OCO create for a resolved pair.
func CreateOcoToProto(symbolID uint32, in models.CreateOcoInput, subAccountID *string, quantityScale int) (*triggersv1.CreateOcoRequest, error) {
	if symbolID == 0 {
		return nil, &errors.ValidationError{Msg: "triggers.create_oco requires a resolved non-zero symbol_id"}
	}
	side, ok := orderSideToProto[strings.ToLower(in.Side)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
	}
	qtyScaled, err := ResolveQtyScaled(in.Qty, quantityScale, "qty", in.Symbol, &symbolID)
	if err != nil {
		return nil, err
	}
	intent := &triggersv1.OcoIntent{SymbolId: symbolID, Side: side, QtyScaled: qtyScaled}
	if in.ClientListID != nil && *in.ClientListID != "" {
		if intent.ClientListId, err = ValidateClientListID(*in.ClientListID); err != nil {
			return nil, err
		}
	} else {
		intent.ClientListId = newTriggerRequestID()
	}
	if in.SelfTradePreventionMode != nil {
		mode, ok := selfTradePreventionModeToProto[strings.ToLower(*in.SelfTradePreventionMode)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "self_trade_prevention_mode must be expire_taker, expire_maker, or expire_both"}
		}
		intent.SelfTradePreventionMode = mode
	}
	if intent.TakeProfit, err = ocoLegToProto(in.TakeProfit, "take_profit", in.Symbol); err != nil {
		return nil, err
	}
	if intent.StopLoss, err = ocoLegToProto(in.StopLoss, "stop_loss", in.Symbol); err != nil {
		return nil, err
	}
	req := &triggersv1.CreateOcoRequest{Oco: intent}
	if req.SubaccountId, err = ParseOptionalSubaccountID(subAccountID); err != nil {
		return nil, err
	}
	return req, nil
}

// ValidateClientListID checks a client OCO ID as the contract does: 1 to 36
// bytes of UTF-8, sent unchanged.
func ValidateClientListID(value string) (string, error) {
	if value == "" || len(value) > clientIDMaxLen || !utf8.ValidString(value) {
		return "", &errors.ValidationError{Msg: fmt.Sprintf("client_list_id must be 1 to %d bytes of UTF-8", clientIDMaxLen)}
	}
	return value, nil
}

func ocoLegToProto(leg models.RiskLegInput, fieldName, symbol string) (*triggersv1.OcoLeg, error) {
	ticks, source, child, err := riskLegToProto(leg, fieldName, symbol)
	if err != nil {
		return nil, err
	}
	return &triggersv1.OcoLeg{TriggerPriceTicks: ticks, Child: child, TriggerPriceSource: source}, nil
}

// BatchCancelTriggersToProto encodes a batch trigger cancel of 1 to
// MaxBatchCancelTriggers distinct trigger IDs. A request ID is generated when
// requestID is nil or empty.
func BatchCancelTriggersToProto(triggerIDs []string, subAccountID *string, requestID *string) (*triggersv1.BatchCancelTriggersRequest, error) {
	if err := validateBatchSize("triggers.batch_cancel", len(triggerIDs), MaxBatchCancelTriggers); err != nil {
		return nil, err
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-cancel-triggers")
	if err != nil {
		return nil, err
	}
	req := &triggersv1.BatchCancelTriggersRequest{RequestId: resolvedRequestID, TriggerIds: make([]uint64, 0, len(triggerIDs))}
	seen := make(map[uint64]struct{}, len(triggerIDs))
	for _, raw := range triggerIDs {
		id, err := IDToInt(raw, "trigger_id")
		if err != nil {
			return nil, err
		}
		if id == 0 {
			return nil, &errors.ValidationError{Msg: "trigger_id must be a non-zero trigger ID"}
		}
		if _, dup := seen[id]; dup {
			return nil, &errors.ValidationError{Msg: "triggers.batch_cancel trigger IDs must be distinct: " + FormatUint64ID(id) + " is repeated"}
		}
		seen[id] = struct{}{}
		req.TriggerIds = append(req.TriggerIds, id)
	}
	if req.SubaccountId, err = ParseOptionalSubaccountID(subAccountID); err != nil {
		return nil, err
	}
	return req, nil
}

func newTriggerRequestID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("trg-%s", hex.EncodeToString(b[:]))
}
