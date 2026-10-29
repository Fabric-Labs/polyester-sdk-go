package codecs

import (
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// RiskPolicyToProto encodes a typed attached-risk input onto the RiskPolicy
// wire used by order create, modify, and batch replace. A nil input encodes
// to nil. fieldName prefixes validation messages.
func RiskPolicyToProto(in *models.AttachedRiskInput, fieldName, symbol string) (*orderv1.RiskPolicy, error) {
	if in == nil {
		return nil, nil
	}
	if in.StopLoss != nil && in.TrailingStop != nil {
		return nil, &errors.ValidationError{Msg: fieldName + " allows at most one of stop_loss or trailing_stop"}
	}
	hasStop := in.StopLoss != nil || in.TrailingStop != nil
	if in.TakeProfit == nil && !hasStop {
		return nil, &errors.ValidationError{Msg: fieldName + " requires take_profit and/or a stop leg"}
	}
	if in.OCO && (in.TakeProfit == nil || !hasStop) {
		return nil, &errors.ValidationError{Msg: fieldName + ".oco requires take_profit and exactly one stop leg (stop_loss or trailing_stop)"}
	}
	policy := &orderv1.RiskPolicy{Oco: in.OCO}
	if in.TakeProfit != nil {
		ticks, source, child, err := riskLegToProto(*in.TakeProfit, fieldName+".take_profit", symbol)
		if err != nil {
			return nil, err
		}
		policy.TakeProfit = &orderv1.TakeProfitPolicy{TriggerPriceTicks: ticks, TriggerPriceSource: source, Child: child}
	}
	if in.StopLoss != nil {
		ticks, source, child, err := riskLegToProto(*in.StopLoss, fieldName+".stop_loss", symbol)
		if err != nil {
			return nil, err
		}
		policy.StopLeg = &orderv1.RiskPolicy_StopLoss{StopLoss: &orderv1.StopLossPolicy{TriggerPriceTicks: ticks, TriggerPriceSource: source, Child: child}}
	}
	if in.TrailingStop != nil {
		trailing, err := trailingStopToProto(*in.TrailingStop, fieldName+".trailing_stop", symbol)
		if err != nil {
			return nil, err
		}
		policy.StopLeg = &orderv1.RiskPolicy_TrailingStop{TrailingStop: trailing}
	}
	return policy, nil
}

func riskLegToProto(leg models.RiskLegInput, fieldName, symbol string) (int64, orderv1.TriggerPriceSource, *orderv1.RiskExecution, error) {
	trigger, err := positivePriceTicks(leg.TriggerPrice, fieldName+".trigger_price", symbol)
	if err != nil {
		return 0, 0, nil, err
	}
	source, err := triggerPriceSourceToProto(leg.TriggerPriceSource, fieldName+".trigger_price_source")
	if err != nil {
		return 0, 0, nil, err
	}
	if leg.LimitPrice == nil {
		return trigger, source, &orderv1.RiskExecution{Execution: &orderv1.RiskExecution_MarketIoc{MarketIoc: &orderv1.RiskMarketIoc{}}}, nil
	}
	limit, err := positivePriceTicks(*leg.LimitPrice, fieldName+".limit_price", symbol)
	if err != nil {
		return 0, 0, nil, err
	}
	return trigger, source, &orderv1.RiskExecution{Execution: &orderv1.RiskExecution_LimitGtc{LimitGtc: &orderv1.RiskLimitGtc{PriceTicks: limit}}}, nil
}

func trailingStopToProto(in models.TrailingStopInput, fieldName, symbol string) (*orderv1.TrailingStopPolicy, error) {
	if (in.DistanceTicks == nil) == (in.DistanceBps == nil) {
		return nil, &errors.ValidationError{Msg: fieldName + " requires exactly one of distance_ticks or distance_bps"}
	}
	if in.MaxSlippageTicks != nil && in.MaxSlippageBps != nil {
		return nil, &errors.ValidationError{Msg: fieldName + " allows at most one of max_slippage_ticks or max_slippage_bps"}
	}
	out := &orderv1.TrailingStopPolicy{}
	if in.DistanceTicks != nil {
		if *in.DistanceTicks <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + ".distance_ticks must be positive"}
		}
		out.TrailingDistance = &orderv1.TrailingStopPolicy_TrailingDistanceTicks{TrailingDistanceTicks: *in.DistanceTicks}
	} else {
		if err := validateBps(fieldName+".distance_bps", *in.DistanceBps, false); err != nil {
			return nil, err
		}
		out.TrailingDistance = &orderv1.TrailingStopPolicy_TrailingDistanceBps{TrailingDistanceBps: *in.DistanceBps}
	}
	if in.MaxSlippageTicks != nil {
		if *in.MaxSlippageTicks <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + ".max_slippage_ticks must be positive"}
		}
		out.MaxSlippage = &orderv1.TrailingStopPolicy_MaxSlippageTicks{MaxSlippageTicks: *in.MaxSlippageTicks}
	}
	if in.MaxSlippageBps != nil {
		if err := validateBps(fieldName+".max_slippage_bps", *in.MaxSlippageBps, false); err != nil {
			return nil, err
		}
		out.MaxSlippage = &orderv1.TrailingStopPolicy_MaxSlippageBps{MaxSlippageBps: *in.MaxSlippageBps}
	}
	if in.ActivationPrice != nil && in.ActivationPrice.IsSet() {
		ticks, err := positivePriceTicks(*in.ActivationPrice, fieldName+".activation_price", symbol)
		if err != nil {
			return nil, err
		}
		out.ActivationPriceTicks = ticks
	}
	source, err := triggerPriceSourceToProto(in.TriggerPriceSource, fieldName+".trigger_price_source")
	if err != nil {
		return nil, err
	}
	out.TriggerPriceSource = source
	return out, nil
}

func positivePriceTicks(in models.PriceInput, fieldName, symbol string) (int64, error) {
	ticks, err := ResolvePriceTicks(in, fieldName, symbol)
	if err != nil {
		return 0, err
	}
	if ticks <= 0 {
		return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
	}
	return ticks, nil
}
