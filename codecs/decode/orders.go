package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// OrderFromProto decodes an order message.
func OrderFromProto(msg *orderv1.Order) models.Order {
	if msg == nil {
		return models.Order{}
	}
	return models.Order{
		OrderID:       codecs.FormatUint64ID(msg.GetOrderId()),
		SymbolID:      msg.GetSymbolId(),
		ClientOrderID: msg.GetClientOrderId(),
		Side:          codecs.OrderSideName(msg.GetSide()),
		Status:        orderStatusName(msg.GetStatus()),
		OrderType:     codecs.OrderTypeName(msg.GetOrderType()),
		TIF:           codecs.TimeInForceName(msg.GetTimeInForce()),
		OrigQty:       strconv.FormatInt(msg.GetOrigQtyScaled(), 10),
		CumQty:        strconv.FormatInt(msg.GetCumQtyScaled(), 10),
		LeavesQty:     strconv.FormatInt(msg.GetLeavesQtyScaled(), 10),
		PriceTicks:    strconv.FormatInt(msg.GetPriceTicks(), 10),
		AvgPxTicks:    strconv.FormatInt(msg.GetAvgPriceTicks(), 10),
		CreatedTsNs:   strconv.FormatUint(msg.GetCreatedTsNs(), 10),
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
	return models.UserTrade{
		SymbolID:   msg.GetSymbolId(),
		MatchID:    strconv.FormatUint(msg.GetMatchId(), 10),
		OrderID:    codecs.FormatUint64ID(msg.GetOrderId()),
		Side:       codecs.OrderSideName(msg.GetSide()),
		IsMaker:    msg.GetIsMaker(),
		PriceTicks: strconv.FormatInt(msg.GetPriceTicks(), 10),
		QtyScaled:  strconv.FormatInt(msg.GetQtyScaled(), 10),
		FeeScaled:  strconv.FormatInt(msg.GetFeeScaled(), 10),
		TsNs:       strconv.FormatUint(msg.GetTsNs(), 10),
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
func OrderMutationFromProto(msg *orderv1.CreateOrderResponse) models.OrderMutationResult {
	return orderMutation(msg.GetStatus(), msg.GetOrderId(), msg.GetClientOrderId())
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
func BatchCreateFromProto(msg *orderv1.BatchCreateOrdersResponse) models.BatchCreateOrdersResult {
	results := make([]models.BatchCreateResultItem, 0, len(msg.GetResults()))
	for _, item := range msg.GetResults() {
		results = append(results, models.BatchCreateResultItem{
			Status:        item.GetStatus(),
			OrderID:       codecs.FormatUint64ID(item.GetOrderId()),
			ClientOrderID: item.GetClientOrderId(),
			Code:          item.GetCode(),
		})
	}
	return models.BatchCreateOrdersResult{
		Results:       results,
		AcceptedCount: int(msg.GetAcceptedCount()),
		RejectedCount: int(msg.GetRejectedCount()),
	}
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
