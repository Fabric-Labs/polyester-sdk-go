package codecs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

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
	TrailingDistanceTicks   *int64
	TrailingDistanceBps     *int32
	ActivationPrice         *models.PriceInput
	MaxSlippageTicks        *int32
	MaxSlippageBps          *int32
	TwapDurationMs          *int64
	TwapSliceIntervalMs     *int64
	LadderPriceMin          *models.PriceInput
	LadderPriceMax          *models.PriceInput
	LadderLevels            *int32
	LadderDistribution      *string
}

func CreateTriggerToProto(symbol, triggerType string, triggerPrice *models.PriceInput, side string, qty models.QtyInput, orderType string, limitPrice *models.PriceInput, triggerPriceSource, tif string, subAccountID *string, clientTriggerID *string, postOnly bool, quantityScale int, opts CreateTriggerOptions) (*triggersv1.CreateTriggerRequest, error) {
	typeKey := strings.ToLower(strings.ReplaceAll(triggerType, "-", "_"))
	if _, ok := triggerTypeToProto[typeKey]; !ok {
		return nil, &errors.ValidationError{Msg: "trigger_type must be stop_loss, take_profit, trailing_stop, twap, or ladder"}
	}
	qtyScaled, err := ResolveQtyScaled(qty, quantityScale, "qty", symbol, nil)
	if err != nil {
		return nil, err
	}
	intent := &triggersv1.TriggerIntent{
		Symbol:    symbol,
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

	// triggerPriceSource is no longer part of the create wire contract; it is
	// evaluated server-side. Accept and ignore for API compatibility.
	_ = triggerPriceSource

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
		child, err := conditionalChildToProto(orderType, tif, limitPrice, symbol, postOnly)
		if err != nil {
			return nil, err
		}
		cond := &triggersv1.ConditionalTrigger{
			TriggerPriceTicks: triggerTicks,
			Side:              sideEnum,
			Child:             child,
		}
		if typeKey == "stop_loss" {
			intent.Strategy = &triggersv1.TriggerIntent_StopLoss{StopLoss: cond}
		} else {
			intent.Strategy = &triggersv1.TriggerIntent_TakeProfit{TakeProfit: cond}
		}
	case "trailing_stop":
		// Standalone trailing stops currently support SELL market-IOC children.
		// Wire TrailingStopTrigger carries side (attached stops may use either).
		sideEnum, ok := orderSideToProto[strings.ToLower(side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		if sideEnum != orderv1.Side_SELL {
			return nil, &errors.ValidationError{Msg: "trailing_stop only supports side=sell"}
		}
		trailing := &triggersv1.TrailingStopTrigger{Side: sideEnum}
		switch {
		case opts.TrailingDistanceTicks != nil:
			trailing.TrailingDistance = &triggersv1.TrailingStopTrigger_TrailingDistanceTicks{TrailingDistanceTicks: *opts.TrailingDistanceTicks}
		case opts.TrailingDistanceBps != nil:
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
		switch {
		case opts.MaxSlippageTicks != nil:
			trailing.MaxSlippage = &triggersv1.TrailingStopTrigger_MaxSlippageTicks{MaxSlippageTicks: *opts.MaxSlippageTicks}
		case opts.MaxSlippageBps != nil:
			trailing.MaxSlippage = &triggersv1.TrailingStopTrigger_MaxSlippageBps{MaxSlippageBps: *opts.MaxSlippageBps}
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
			twap.Execution = &triggersv1.TwapTrigger_MarketIoc{MarketIoc: &triggersv1.TwapMarketIoc{}}
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
		if opts.LadderPriceMin != nil && opts.LadderPriceMin.IsSet() {
			ticks, err := ResolvePriceTicks(*opts.LadderPriceMin, "ladder_price_min", symbol)
			if err != nil {
				return nil, err
			}
			ladder.PriceMinTicks = ticks
		}
		if opts.LadderPriceMax != nil && opts.LadderPriceMax.IsSet() {
			ticks, err := ResolvePriceTicks(*opts.LadderPriceMax, "ladder_price_max", symbol)
			if err != nil {
				return nil, err
			}
			ladder.PriceMaxTicks = ticks
		}
		if opts.LadderLevels != nil {
			ladder.Levels = *opts.LadderLevels
		}
		intent.Strategy = &triggersv1.TriggerIntent_Ladder{Ladder: ladder}
	}

	req := &triggersv1.CreateTriggerRequest{Trigger: intent}
	if subAccountID != nil {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		req.SubaccountId = &sub
	}
	return req, nil
}

// conditionalChildToProto maps flat (order_type, tif, limit_price, post_only)
// params onto a stop-loss / take-profit child execution variant.
func conditionalChildToProto(orderType, tif string, limitPrice *models.PriceInput, symbol string, postOnly bool) (*triggersv1.ConditionalChildExecution, error) {
	switch strings.ToLower(orderType) {
	case "market":
		if postOnly {
			return nil, &errors.ValidationError{Msg: "post_only is only valid for limit GTC executions"}
		}
		return &triggersv1.ConditionalChildExecution{
			Execution: &triggersv1.ConditionalChildExecution_MarketIoc{MarketIoc: &triggersv1.TriggerMarketIoc{}},
		}, nil
	case "limit":
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

// ModifyTriggerOptions carries optional trigger patch fields.
type ModifyTriggerOptions struct {
	TriggerPrice          *models.PriceInput
	LimitPrice            *models.PriceInput
	TrailingDistanceTicks *int64
	TrailingDistanceBps   *int32
	ActivationPrice       *models.PriceInput
	MaxSlippageTicks      *int32
	MaxSlippageBps        *int32
}

func ModifyTriggerToProto(triggerID string, subAccountID *string, opts ModifyTriggerOptions) (*triggersv1.ModifyTriggerRequest, error) {
	if (opts.TriggerPrice == nil || !opts.TriggerPrice.IsSet()) &&
		(opts.LimitPrice == nil || !opts.LimitPrice.IsSet()) &&
		opts.TrailingDistanceTicks == nil && opts.TrailingDistanceBps == nil &&
		(opts.ActivationPrice == nil || !opts.ActivationPrice.IsSet()) &&
		opts.MaxSlippageTicks == nil && opts.MaxSlippageBps == nil {
		return nil, &errors.ValidationError{Msg: "modify requires at least one of trigger_price, limit_price, trailing_distance_ticks, trailing_distance_bps, activation_price, max_slippage_ticks, or max_slippage_bps"}
	}
	id, err := IDToInt(triggerID, "trigger_id")
	if err != nil {
		return nil, err
	}
	req := &triggersv1.ModifyTriggerRequest{TriggerId: id}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		req.SubaccountId = &sub
	}
	if opts.TriggerPrice != nil && opts.TriggerPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*opts.TriggerPrice, "trigger_price", "")
		if err != nil {
			return nil, err
		}
		req.TriggerPriceTicks = &ticks
	}
	if opts.LimitPrice != nil && opts.LimitPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*opts.LimitPrice, "limit_price", "")
		if err != nil {
			return nil, err
		}
		req.LimitPriceTicks = &ticks
	}
	if opts.TrailingDistanceTicks != nil {
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceTicks{TrailingDistanceTicks: *opts.TrailingDistanceTicks}
	}
	if opts.TrailingDistanceBps != nil {
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceBps{TrailingDistanceBps: *opts.TrailingDistanceBps}
	}
	if opts.ActivationPrice != nil && opts.ActivationPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*opts.ActivationPrice, "activation_price", "")
		if err != nil {
			return nil, err
		}
		req.ActivationPriceTicks = &ticks
	}
	if opts.MaxSlippageTicks != nil {
		req.MaxSlippage = &triggersv1.ModifyTriggerRequest_MaxSlippageTicks{MaxSlippageTicks: *opts.MaxSlippageTicks}
	}
	if opts.MaxSlippageBps != nil {
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

func newTriggerRequestID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("trg-%s", hex.EncodeToString(b[:]))
}
