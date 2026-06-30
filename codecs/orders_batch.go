package codecs

import (
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"strings"
)

// BatchCreateOrdersToProto encodes a batch create request.
func BatchCreateOrdersToProto(items []models.CreateOrderRequest, subAccountID *string, requestID *string, allowPartial bool, quantityScale int) (*orderv1.BatchCreateOrdersRequest, error) {
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_create requires at least one item"}
	}
	proto := &orderv1.BatchCreateOrdersRequest{
		RequestId:    coalesceRequestID(requestID, "batch-create"),
		AllowPartial: allowPartial,
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	for _, item := range items {
		encoded, err := CreateOrderToProto(item, quantityScale)
		if err != nil {
			return nil, err
		}
		proto.Items = append(proto.Items, encoded)
	}
	return proto, nil
}

// BatchCancelItemToProto encodes one batch cancel item.
func BatchCancelItemToProto(item models.BatchCancelItem) (*orderv1.BatchCancelItem, error) {
	hasOrderID := item.OrderID != nil && *item.OrderID != ""
	hasClient := item.ClientOrderID != nil && *item.ClientOrderID != ""
	if hasOrderID == hasClient {
		return nil, &errors.ValidationError{Msg: "each batch cancel item requires exactly one of order_id or client_order_id"}
	}
	proto := &orderv1.BatchCancelItem{}
	if hasOrderID {
		id, err := IDToInt(*item.OrderID, "order_id")
		if err != nil {
			return nil, err
		}
		proto.OrderId = id
	}
	if hasClient {
		proto.ClientOrderId = *item.ClientOrderID
	}
	if item.SymbolID != nil {
		proto.SymbolId = *item.SymbolID
	}
	return proto, nil
}

// BatchCancelOrdersToProto encodes a batch cancel request.
func BatchCancelOrdersToProto(items []models.BatchCancelItem, subAccountID *string, requestID *string) (*orderv1.BatchCancelOrdersRequest, error) {
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_cancel requires at least one item"}
	}
	proto := &orderv1.BatchCancelOrdersRequest{
		RequestId: coalesceRequestID(requestID, "batch-cancel"),
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	for _, item := range items {
		encoded, err := BatchCancelItemToProto(item)
		if err != nil {
			return nil, err
		}
		proto.Items = append(proto.Items, encoded)
	}
	return proto, nil
}

// BatchModifyItemToProto encodes one batch modify item.
func BatchModifyItemToProto(item models.BatchModifyItem, quantityScale int) (*orderv1.BatchModifyItem, error) {
	hasOrderID := item.OrderID != nil && *item.OrderID != ""
	hasClient := item.ClientOrderID != nil && *item.ClientOrderID != ""
	if hasOrderID == hasClient {
		return nil, &errors.ValidationError{Msg: "each batch item requires exactly one of order_id or client_order_id"}
	}
	if item.NewPrice == nil && item.NewQty == nil {
		return nil, &errors.ValidationError{Msg: "each batch item requires new_price and/or new_qty"}
	}
	proto := &orderv1.BatchModifyItem{}
	if hasOrderID {
		id, err := IDToInt(*item.OrderID, "order_id")
		if err != nil {
			return nil, err
		}
		proto.Key = &orderv1.BatchModifyItem_OrderId{OrderId: id}
	}
	if hasClient {
		proto.Key = &orderv1.BatchModifyItem_ClientOrderId{ClientOrderId: *item.ClientOrderID}
	}
	if item.NewPrice != nil {
		ticks, err := ParsePriceTicks(*item.NewPrice, "new_price")
		if err != nil {
			return nil, err
		}
		v := int64(ticks)
		proto.NewPriceTicks = &v
	}
	if item.NewQty != nil {
		qty, err := ParseQtyScaled(*item.NewQty, quantityScale, "new_qty")
		if err != nil {
			return nil, err
		}
		v := int64(qty)
		proto.NewQtyScaled = &v
	}
	if item.Behavior != nil {
		b, ok := modifyBehaviorToProto[strings.ToLower(*item.Behavior)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "behavior must be amend_or_replace, amend_only, or replace_only"}
		}
		proto.Behavior = b
	}
	if item.NewClientOrderID != nil {
		proto.NewClientOrderId = *item.NewClientOrderID
	}
	return proto, nil
}

// BatchModifyOrdersToProto encodes a batch modify request.
func BatchModifyOrdersToProto(items []models.BatchModifyItem, subAccountID *string, requestID *string, behaviorDefault *string, allowPartial bool, quantityScale int) (*orderv1.BatchModifyOrdersRequest, error) {
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_modify requires at least one item"}
	}
	proto := &orderv1.BatchModifyOrdersRequest{
		RequestId:    coalesceRequestID(requestID, "batch-mod"),
		AllowPartial: allowPartial,
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if behaviorDefault != nil && *behaviorDefault != "" {
		b, ok := modifyBehaviorToProto[strings.ToLower(*behaviorDefault)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "behavior_default must be amend_or_replace, amend_only, or replace_only"}
		}
		proto.BehaviorDefault = b
	}
	for _, item := range items {
		encoded, err := BatchModifyItemToProto(item, quantityScale)
		if err != nil {
			return nil, err
		}
		proto.Items = append(proto.Items, encoded)
	}
	return proto, nil
}
