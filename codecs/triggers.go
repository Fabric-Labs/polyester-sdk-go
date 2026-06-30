package codecs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

var triggerTypeToProto = map[string]triggersv1.TriggerType{
	"stop_loss":     triggersv1.TriggerType_STOP_LOSS,
	"take_profit":   triggersv1.TriggerType_TAKE_PROFIT,
	"trailing_stop": triggersv1.TriggerType_TRAILING_STOP,
	"twap":          triggersv1.TriggerType_TWAP,
	"ladder":        triggersv1.TriggerType_LADDER,
}

var triggerPriceSourceToProto = map[string]orderv1.TriggerPriceSource{
	"last":        orderv1.TriggerPriceSource_LAST_PRICE,
	"last_price":  orderv1.TriggerPriceSource_LAST_PRICE,
	"index":       orderv1.TriggerPriceSource_INDEX_PRICE,
	"index_price": orderv1.TriggerPriceSource_INDEX_PRICE,
	"mark":        orderv1.TriggerPriceSource_MARK_PRICE,
	"mark_price":  orderv1.TriggerPriceSource_MARK_PRICE,
}

func CreateTriggerToProto(symbol, triggerType, triggerPrice, side, qty, orderType string, limitPrice *string, triggerPriceSource, tif string, subAccountID *string, clientTriggerID *string, postOnly bool, quantityScale int) (*triggersv1.CreateTriggerRequest, error) {
	typeKey := strings.ToLower(strings.ReplaceAll(triggerType, "-", "_"))
	triggerEnum, ok := triggerTypeToProto[typeKey]
	if !ok {
		return nil, &errors.ValidationError{Msg: "trigger_type must be stop_loss, take_profit, trailing_stop, twap, or ladder"}
	}
	sideEnum, ok := orderSideToProto[strings.ToLower(side)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
	}
	orderKey := strings.ToLower(orderType)
	orderEnum, ok := orderTypeToProto[orderKey]
	if !ok {
		return nil, &errors.ValidationError{Msg: "order_type must be limit or market"}
	}
	priceTicks, err := ParsePriceTicks(triggerPrice, "trigger_price")
	if err != nil {
		return nil, err
	}
	qtyScaled, err := ParseQtyScaled(qty, quantityScale, "qty")
	if err != nil {
		return nil, err
	}
	req := &triggersv1.CreateTriggerRequest{
		Symbol:            symbol,
		TriggerType:       triggerEnum,
		TriggerPriceTicks: int64(priceTicks),
		Side:              sideEnum,
		OrderType:         orderEnum,
		QtyScaled:         int64(qtyScaled),
		PostOnly:          postOnly,
	}
	if source, ok := triggerPriceSourceToProto[strings.ToLower(triggerPriceSource)]; ok {
		req.TriggerPriceSource = source
	}
	if tifKey := strings.ToLower(tif); tifKey != "" {
		if tifEnum, ok := tifToProto[tifKey]; ok {
			req.TimeInForce = tifEnum
		}
	}
	if subAccountID != nil {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		req.SubaccountId = &sub
	}
	if clientTriggerID != nil && *clientTriggerID != "" {
		req.ClientTriggerId = *clientTriggerID
	} else {
		req.ClientTriggerId = newTriggerRequestID()
	}
	if limitPrice != nil {
		ticks, err := ParsePriceTicks(*limitPrice, "limit_price")
		if err != nil {
			return nil, err
		}
		req.LimitPriceTicks = int64(ticks)
	}
	return req, nil
}

// ModifyTriggerOptions carries optional trigger patch fields.
type ModifyTriggerOptions struct {
	TriggerPrice          *string
	LimitPrice            *string
	TrailingDistanceTicks *int64
	TrailingDistanceBps   *int32
	ActivationPrice       *string
	MaxSlippageTicks      *int32
	MaxSlippageBps        *int32
}

func ModifyTriggerToProto(triggerID string, subAccountID *string, opts ModifyTriggerOptions) (*triggersv1.ModifyTriggerRequest, error) {
	if opts.TriggerPrice == nil && opts.LimitPrice == nil &&
		opts.TrailingDistanceTicks == nil && opts.TrailingDistanceBps == nil &&
		opts.ActivationPrice == nil && opts.MaxSlippageTicks == nil && opts.MaxSlippageBps == nil {
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
	if opts.TriggerPrice != nil {
		ticks, err := ParsePriceTicks(*opts.TriggerPrice, "trigger_price")
		if err != nil {
			return nil, err
		}
		v := int64(ticks)
		req.TriggerPriceTicks = &v
	}
	if opts.LimitPrice != nil {
		ticks, err := ParsePriceTicks(*opts.LimitPrice, "limit_price")
		if err != nil {
			return nil, err
		}
		v := int64(ticks)
		req.LimitPriceTicks = &v
	}
	if opts.TrailingDistanceTicks != nil {
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceTicks{TrailingDistanceTicks: *opts.TrailingDistanceTicks}
	}
	if opts.TrailingDistanceBps != nil {
		req.TrailingDistance = &triggersv1.ModifyTriggerRequest_TrailingDistanceBps{TrailingDistanceBps: *opts.TrailingDistanceBps}
	}
	if opts.ActivationPrice != nil {
		ticks, err := ParsePriceTicks(*opts.ActivationPrice, "activation_price")
		if err != nil {
			return nil, err
		}
		v := int64(ticks)
		req.ActivationPriceTicks = &v
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
