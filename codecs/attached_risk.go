package codecs

import (
	"strings"

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
		msg := fieldName + " requires take_profit and/or a stop leg"
		if fieldName == "new_attached_risk" {
			msg += "; to remove the order's attached risk, set ClearAttachedRisk instead"
		}
		return nil, &errors.ValidationError{Msg: msg}
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
	// A leg's prices are refused with the codes the Orders API answers for
	// them: ERROR_CODE_TRIGGER_PRICE_INVALID and ERROR_CODE_BAD_PRICE.
	trigger, err := positivePriceTicks(leg.TriggerPrice, fieldName+".trigger_price", symbol)
	if err != nil {
		return 0, 0, nil, coded(err, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID)
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
		return 0, 0, nil, coded(err, orderv1.ErrorCode_ERROR_CODE_BAD_PRICE)
	}
	return trigger, source, &orderv1.RiskExecution{Execution: &orderv1.RiskExecution_LimitGtc{LimitGtc: &orderv1.RiskLimitGtc{PriceTicks: limit}}}, nil
}

// trailingStopToProto encodes an attached trailing stop, refusing its
// distance, slippage cap and activation price with the codes a standalone
// trailing stop answers for the same mistake, as the API answers them on
// every transport.
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
			return nil, &errors.ValidationError{Msg: fieldName + ".distance_ticks must be positive", Code: orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID.String()}
		}
		out.TrailingDistance = &orderv1.TrailingStopPolicy_TrailingDistanceTicks{TrailingDistanceTicks: *in.DistanceTicks}
	} else {
		if err := validateBps(fieldName+".distance_bps", *in.DistanceBps, false); err != nil {
			return nil, coded(err, orderv1.ErrorCode_ERROR_CODE_TRAILING_DISTANCE_INVALID)
		}
		out.TrailingDistance = &orderv1.TrailingStopPolicy_TrailingDistanceBps{TrailingDistanceBps: *in.DistanceBps}
	}
	if in.MaxSlippageTicks != nil {
		if *in.MaxSlippageTicks <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + ".max_slippage_ticks must be positive", Code: orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_TICKS_INVALID.String()}
		}
		out.MaxSlippage = &orderv1.TrailingStopPolicy_MaxSlippageTicks{MaxSlippageTicks: *in.MaxSlippageTicks}
	}
	if in.MaxSlippageBps != nil {
		// An attached trailing stop takes no 100% cap on either parent side.
		if err := validateBpsAtMost(fieldName+".max_slippage_bps", *in.MaxSlippageBps, false, MaxSellingSlippageBps); err != nil {
			// Not positive: SLIPPAGE_BPS_INVALID; 10000 or more:
			// MAX_SLIPPAGE_INVALID.
			code := orderv1.ErrorCode_ERROR_CODE_MAX_SLIPPAGE_INVALID
			if *in.MaxSlippageBps < 1 {
				code = orderv1.ErrorCode_ERROR_CODE_SLIPPAGE_BPS_INVALID
			}
			return nil, coded(err, code)
		}
		out.MaxSlippage = &orderv1.TrailingStopPolicy_MaxSlippageBps{MaxSlippageBps: *in.MaxSlippageBps}
	}
	if in.ActivationPrice != nil && in.ActivationPrice.IsSet() {
		ticks, err := positivePriceTicks(*in.ActivationPrice, fieldName+".activation_price", symbol)
		if err != nil {
			return nil, coded(err, orderv1.ErrorCode_ERROR_CODE_TRIGGER_ACTIVATION_PRICE_INVALID)
		}
		out.ActivationPriceTicks = ticks
	}
	if in.LimitPrice != nil && in.LimitPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*in.LimitPrice, fieldName+".limit_price", symbol)
		if err != nil {
			return nil, err
		}
		// A zero maximum is no maximum on the wire. Where the parent's side is
		// known (create, preview, batch create) the side rule then decides: a
		// selling parent needs one (TRIGGER_BUY_REQUIRES_LIMIT, buy_required).
		sideRuleDecides := ticks == 0 && strings.HasPrefix(fieldName, "attached_risk.")
		switch {
		case sideRuleDecides:
		case ticks <= 0:
			// Modify and batch replace don't know the parent's side. Name both
			// sides of the contract's rule instead of a bare positivity check.
			return nil, trailingLimitError(fieldName, orderv1.ErrorCode_ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT, "buy_required",
				fieldName+".limit_price must be positive: it is the maximum price the buy-back pays, required when the parent order sells; leave it unset when the parent order buys")
		case out.ActivationPriceTicks > ticks:
			return nil, trailingLimitError(fieldName, orderv1.ErrorCode_ERROR_CODE_BAD_PRICE, "buy_at_least_activation",
				fieldName+".limit_price must be at least activation_price")
		default:
			out.LimitPriceTicks = ticks
		}
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

// validateTrailingLimitForSide applies the parent-side rule of an attached
// trailing stop's buy-back limit: a sell parent's trailing stop buys back and
// needs its maximum price, while a buy parent's sells and takes none. Paths
// that do not know the parent's side leave this rule to the server.
func validateTrailingLimitForSide(policy *orderv1.RiskPolicy, side orderv1.Side, fieldName string) error {
	trailing := policy.GetTrailingStop()
	if trailing == nil {
		return nil
	}
	switch {
	case side == orderv1.Side_SELL && trailing.GetLimitPriceTicks() == 0:
		return trailingLimitError(fieldName+".trailing_stop", orderv1.ErrorCode_ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT, "buy_required",
			fieldName+".trailing_stop.limit_price is required when the parent order sells: the maximum price its buy-back pays")
	case side == orderv1.Side_BUY && trailing.GetLimitPriceTicks() != 0:
		return trailingLimitError(fieldName+".trailing_stop", orderv1.ErrorCode_ERROR_CODE_BAD_PRICE, "sell_unset",
			fieldName+".trailing_stop.limit_price must be unset when the parent order buys")
	}
	return nil
}

// trailingLimitError refuses an attached trailing stop's buy-back maximum with
// the code, rule ID and Connect field path the server answers for it.
// fieldName is "attached_risk.trailing_stop" (create, preview, batch create:
// the field sits under the request's order) or
// "new_attached_risk.trailing_stop" (modify, batch replace).
func trailingLimitError(fieldName string, code orderv1.ErrorCode, ruleID, msg string) error {
	path := fieldName + ".limit_price_ticks"
	if strings.HasPrefix(fieldName, "attached_risk.") {
		path = "order." + path
	}
	violations := []errors.FieldViolation{{FieldPath: path, RuleID: ruleID, Message: msg}}
	return &errors.ValidationError{
		Msg:        msg,
		Code:       code.String(),
		Metadata:   errors.ViolationMetadata(violations),
		Violations: violations,
	}
}

// ReplacementRiskPolicyToProto encodes the new_attached_risk of a modify or a
// batch replace item. With clear it is an empty RiskPolicy, which removes the
// order's take-profit, stop-loss and trailing-stop legs; clear with a non-nil
// newRisk is refused. Without clear, newRisk is encoded as RiskPolicyToProto
// does: nil leaves the legs unchanged, and an empty one is refused.
func ReplacementRiskPolicyToProto(newRisk *models.AttachedRiskInput, clear bool, symbol string) (*orderv1.RiskPolicy, error) {
	return replacementRiskPolicyToProto(newRisk, clear, symbol, "new_attached_risk")
}

// replacementRiskPolicyToProto is ReplacementRiskPolicyToProto with the
// field path refusals name: "new_attached_risk" for a modify or a single
// item, "items[i].new_attached_risk" for an item of a whole batch request.
func replacementRiskPolicyToProto(newRisk *models.AttachedRiskInput, clear bool, symbol, riskField string) (*orderv1.RiskPolicy, error) {
	if !clear {
		return RiskPolicyToProto(newRisk, riskField, symbol)
	}
	if newRisk != nil {
		return nil, &errors.ValidationError{Msg: "set " + riskField + " or clear_attached_risk, not both"}
	}
	return &orderv1.RiskPolicy{}, nil
}
