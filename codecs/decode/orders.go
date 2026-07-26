package decode

import (
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// OrderFromProto decodes an order message.
func OrderFromProto(msg *orderv1.Order) models.Order {
	if msg == nil {
		return models.Order{}
	}
	sid := msg.GetSymbolId()
	return models.Order{
		OrderID:       codecs.FormatUint64ID(msg.GetOrderId()),
		SymbolID:      sid,
		ClientOrderID: msg.GetClientOrderId(),
		Side:          codecs.OrderSideName(msg.GetSide()),
		Status:        orderStatusName(msg.GetStatus()),
		OrderType:     codecs.OrderTypeName(msg.GetOrderType()),
		TIF:           codecs.TimeInForceName(msg.GetTimeInForce()),
		OrigQty:       codecs.DecodeQtyScaled(msg.GetOrigQtyScaled(), -1, "", &sid),
		CumQty:        codecs.DecodeQtyScaled(msg.GetCumQtyScaled(), -1, "", &sid),
		LeavesQty:     codecs.DecodeQtyScaled(msg.GetLeavesQtyScaled(), -1, "", &sid),
		Price:         codecs.DecodePriceTicks(msg.GetPriceTicks(), ""),
		AvgPx:         codecs.DecodePriceTicks(msg.GetAvgPriceTicks(), ""),
		CreatedTsNs:   strconv.FormatUint(msg.GetCreatedTsNs(), 10),
		Version:       msg.GetVersion(),
		PostOnly:      msg.GetPostOnly(),
		AttachedRisk:  attachedRiskFromProto(msg.GetAttachedRisk()),
	}
}

// riskLegFromChild projects an attached take-profit/stop-loss policy onto the
// flat public RiskLeg. The child execution determines order_type/limit_price.
// TriggerPriceSource is no longer part of the policy wire and is left empty.
func riskLegFromChild(triggerPriceTicks int64, child *orderv1.RiskExecution) *models.RiskLeg {
	if triggerPriceTicks == 0 {
		return nil
	}
	leg := &models.RiskLeg{
		TriggerPrice: codecs.DecodePriceTicks(triggerPriceTicks, ""),
	}
	if child != nil {
		switch {
		case child.GetMarketIoc() != nil:
			leg.OrderType = "market"
		case child.GetLimitGtc() != nil:
			leg.OrderType = "limit"
			leg.LimitPrice = codecs.DecodePriceTicks(child.GetLimitGtc().GetPriceTicks(), "")
		}
	}
	return leg
}

func riskLegFromTakeProfit(policy *orderv1.TakeProfitPolicy) *models.RiskLeg {
	if policy == nil {
		return nil
	}
	return riskLegFromChild(policy.GetTriggerPriceTicks(), policy.GetChild())
}

func riskLegFromStopLoss(policy *orderv1.StopLossPolicy) *models.RiskLeg {
	if policy == nil {
		return nil
	}
	return riskLegFromChild(policy.GetTriggerPriceTicks(), policy.GetChild())
}

func trailingStopFromPolicy(policy *orderv1.TrailingStopPolicy) *models.TrailingStop {
	if policy == nil {
		return nil
	}
	// OrderType/TriggerPriceSource were dropped from the trailing-stop policy
	// wire; the child is an implicit market execution.
	out := &models.TrailingStop{
		DistanceTicks:    policy.GetTrailingDistanceTicks(),
		DistanceBps:      policy.GetTrailingDistanceBps(),
		MaxSlippageTicks: policy.GetMaxSlippageTicks(),
		MaxSlippageBps:   policy.GetMaxSlippageBps(),
	}
	if policy.GetActivationPriceTicks() > 0 {
		out.ActivationPrice = codecs.DecodePriceTicks(policy.GetActivationPriceTicks(), "")
	}
	return out
}

func attachedRiskFromProto(msg *orderv1.AttachedRisk) *models.AttachedRisk {
	if msg == nil {
		return nil
	}
	var takeProfit *models.RiskLeg
	if tp := msg.GetTakeProfit(); tp != nil {
		takeProfit = riskLegFromTakeProfit(tp.GetPolicy())
	}
	var trailingStop *models.TrailingStop
	if ts := msg.GetTrailingStop(); ts != nil {
		trailingStop = trailingStopFromPolicy(ts.GetPolicy())
	}
	var stopLoss *models.RiskLeg
	// Match TS: when trailing is present, stop-loss is suppressed.
	if trailingStop == nil {
		if sl := msg.GetStopLoss(); sl != nil {
			stopLoss = riskLegFromStopLoss(sl.GetPolicy())
		}
	}
	if takeProfit == nil && stopLoss == nil && trailingStop == nil {
		return nil
	}
	return &models.AttachedRisk{
		TakeProfit:   takeProfit,
		StopLoss:     stopLoss,
		TrailingStop: trailingStop,
		Oco:          msg.GetOco(),
	}
}

func orderStatusName(v orderv1.OrderStatus) string {
	switch v {
	case orderv1.OrderStatus_PENDING:
		return "pending"
	case orderv1.OrderStatus_PENDING_CANCEL:
		return "pending_cancel"
	case orderv1.OrderStatus_WORKING:
		return "working"
	case orderv1.OrderStatus_FILLED:
		return "filled"
	case orderv1.OrderStatus_CANCELED:
		return "canceled"
	case orderv1.OrderStatus_REJECTED:
		return "rejected"
	default:
		return ""
	}
}

// OrdersListFromOpen decodes open orders response.
func OrdersListFromOpen(msg *orderv1.GetOpenOrdersResponse) models.OrdersList {
	return ordersList(msg.GetOrders(), msg.GetNextPageToken())
}

// OrdersListFromHistory decodes order history response.
func OrdersListFromHistory(msg *orderv1.GetOrderHistoryResponse) models.OrdersList {
	return ordersList(msg.GetOrders(), msg.GetNextPageToken())
}

func ordersList(orders []*orderv1.Order, token string) models.OrdersList {
	out := make([]models.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, OrderFromProto(o))
	}
	return models.OrdersList{Orders: out, NextPageToken: token}
}

// UserTradeFromProto decodes a user trade.
func UserTradeFromProto(msg *orderv1.UserTrade) models.UserTrade {
	if msg == nil {
		return models.UserTrade{}
	}
	sid := msg.GetSymbolId()
	return models.UserTrade{
		SymbolID:  sid,
		MatchID:   strconv.FormatUint(msg.GetMatchId(), 10),
		OrderID:   codecs.FormatUint64ID(msg.GetOrderId()),
		Side:      codecs.OrderSideName(msg.GetSide()),
		IsMaker:   msg.GetIsMaker(),
		Price:     codecs.DecodePriceTicks(msg.GetPriceTicks(), ""),
		Qty:       codecs.DecodeQtyScaled(msg.GetQtyScaled(), -1, "", &sid),
		FeeScaled: strconv.FormatInt(msg.GetFeeScaled(), 10),
		TsNs:      strconv.FormatUint(msg.GetTsNs(), 10),
	}
}

// UserTradesListFromProto decodes user trades list.
func UserTradesListFromProto(msg *orderv1.GetUserTradesResponse) models.UserTradesList {
	trades := make([]models.UserTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		trades = append(trades, UserTradeFromProto(t))
	}
	return models.UserTradesList{Trades: trades, NextPageToken: msg.GetNextPageToken()}
}

// GetOrderFromProto decodes get order response.
func GetOrderFromProto(msg *orderv1.GetOrderResponse) models.GetOrderResult {
	var order *models.Order
	if msg.GetOrder() != nil {
		o := OrderFromProto(msg.GetOrder())
		order = &o
	}
	trades := make([]models.UserTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		trades = append(trades, UserTradeFromProto(t))
	}
	return models.GetOrderResult{Order: order, Trades: trades}
}

// OrderMutationFromProto decodes order mutation response.
//
// CreateOrderResponse acknowledges admission only and no longer carries a
// status field; synthesize "accepted".
func OrderMutationFromProto(msg *orderv1.CreateOrderResponse) models.OrderMutationResult {
	return orderMutation("accepted", msg.GetOrderId(), msg.GetClientOrderId())
}

// OrderMutationFromCancel decodes cancel response.
func OrderMutationFromCancel(msg *orderv1.CancelOrderResponse) models.OrderMutationResult {
	return orderMutation(msg.GetStatus(), msg.GetOrderId(), "")
}

func orderMutation(status string, orderID uint64, clientOrderID string) models.OrderMutationResult {
	return models.OrderMutationResult{
		Status:        status,
		OrderID:       codecs.FormatUint64ID(orderID),
		ClientOrderID: clientOrderID,
	}
}

// ModifyOrderFromProto decodes modify order response.
func ModifyOrderFromProto(msg *orderv1.ModifyOrderResponse) models.ModifyOrderResult {
	return models.ModifyOrderResult{
		ActionTaken:  modifyActionName(msg.GetActionTaken()),
		OldOrderID:   codecs.FormatUint64ID(msg.GetOldOrderId()),
		FinalOrderID: codecs.FormatUint64ID(msg.GetFinalOrderId()),
		Code:         msg.GetCode(),
	}
}

func modifyActionName(v orderv1.ModifyActionTaken) string {
	switch v {
	case orderv1.ModifyActionTaken_AMENDED:
		return "amended"
	case orderv1.ModifyActionTaken_REPLACED:
		return "replaced"
	default:
		return ""
	}
}

// CancelAllFromProto decodes cancel all response.
func CancelAllFromProto(msg *orderv1.CancelAllOrdersResponse) models.CancelAllOrdersResult {
	return models.CancelAllOrdersResult{
		Status:           msg.GetStatus(),
		MatchedOrders:    int(msg.GetMatchedOrders()),
		SubmittedCancels: int(msg.GetSubmittedCancels()),
	}
}

// BatchModifyFromProto decodes batch modify response.
func BatchModifyFromProto(msg *orderv1.BatchModifyOrdersResponse) models.BatchModifyOrdersResult {
	results := make([]models.BatchModifyResultItem, 0, len(msg.GetResults()))
	for _, item := range msg.GetResults() {
		results = append(results, models.BatchModifyResultItem{
			Status:        item.GetStatus(),
			ClientOrderID: item.GetClientOrderId(),
			FinalOrderID:  codecs.FormatUint64ID(item.GetFinalOrderId()),
			Code:          item.GetCode(),
		})
	}
	return models.BatchModifyOrdersResult{
		Results:       results,
		AmendedCount:  int(msg.GetAmendedCount()),
		ReplacedCount: int(msg.GetReplacedCount()),
		RejectedCount: int(msg.GetRejectedCount()),
	}
}

// BatchCreateFromProto decodes batch create response.
//
// Per-item results now carry an Accepted/Rejected outcome oneof instead of flat
// status/order_id/code fields.
func BatchCreateFromProto(msg *orderv1.BatchCreateOrdersResponse) (models.BatchCreateOrdersResult, error) {
	results := make([]models.BatchCreateResultItem, 0, len(msg.GetResults()))
	decodedAccepted := 0
	decodedRejected := 0
	for _, item := range msg.GetResults() {
		out := models.BatchCreateResultItem{
			ClientOrderID: item.GetClientOrderId(),
		}
		if accepted := item.GetAccepted(); accepted != nil {
			decodedAccepted++
			out.Status = "accepted"
			out.OrderID = codecs.FormatUint64ID(accepted.GetOrderId())
		} else if rejected := item.GetRejected(); rejected != nil {
			decodedRejected++
			out.Status = "rejected"
			if err := rejected.GetError(); err != nil {
				raw := int32(err.GetCode())
				if raw == 0 {
					out.Code = "ERROR_CODE_UNSPECIFIED"
				} else if name, ok := orderv1.ErrorCode_name[raw]; ok {
					out.Code = name
				} else {
					out.Code = fmt.Sprintf("UNKNOWN_ERROR_CODE(%d)", raw)
				}
			} else {
				out.Code = "ERROR_CODE_UNSPECIFIED"
			}
		} else {
			return models.BatchCreateOrdersResult{}, &sdkerrors.TransportError{
				Msg: "batch create response item has neither accepted nor rejected outcome",
			}
		}
		results = append(results, out)
	}
	accepted := int(msg.GetAcceptedCount())
	rejected := int(msg.GetRejectedCount())
	if accepted != decodedAccepted || rejected != decodedRejected ||
		accepted+rejected != len(results) {
		return models.BatchCreateOrdersResult{}, &sdkerrors.TransportError{
			Msg: fmt.Sprintf(
				"batch create response counts do not match decoded outcomes: accepted=%d/%d rejected=%d/%d results=%d",
				accepted, decodedAccepted, rejected, decodedRejected, len(results),
			),
		}
	}
	return models.BatchCreateOrdersResult{
		Results:       results,
		AcceptedCount: accepted,
		RejectedCount: rejected,
	}, nil
}

// BatchCancelFromProto decodes batch cancel response.
func BatchCancelFromProto(msg *orderv1.BatchCancelOrdersResponse) models.BatchCancelOrdersResult {
	results := make([]models.BatchCancelResultItem, 0, len(msg.GetResults()))
	for _, item := range msg.GetResults() {
		results = append(results, models.BatchCancelResultItem{
			Status:        item.GetStatus(),
			OrderID:       codecs.FormatUint64ID(item.GetOrderId()),
			ClientOrderID: item.GetClientOrderId(),
			Code:          item.GetCode(),
		})
	}
	return models.BatchCancelOrdersResult{
		Results:       results,
		AcceptedCount: int(msg.GetAcceptedCount()),
		RejectedCount: int(msg.GetRejectedCount()),
	}
}

// CancelAllAfterFromProto decodes cancel-all-after response.
func CancelAllAfterFromProto(msg *orderv1.CancelAllAfterResponse) models.CancelAllAfterResult {
	return models.CancelAllAfterResult{
		Status:              msg.GetStatus(),
		EffectiveTimeoutSec: int(msg.GetEffectiveTimeoutSec()),
		ExpiresAtTsNs:       strconv.FormatUint(msg.GetExpiresAtTsNs(), 10),
	}
}
