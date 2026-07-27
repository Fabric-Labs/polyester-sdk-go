package codecs

import (
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"strings"
)

// BatchCreateOrdersToProto encodes a batch create request.
//
// The allowPartial argument is retained for API compatibility but is ignored:
// the POLY-3701 wire contract removed the batch allow_partial field.
func BatchCreateOrdersToProto(items []models.CreateOrderRequest, subAccountID *string, requestID *string, allowPartial bool, quantityScale int) (*orderv1.BatchCreateOrdersRequest, error) {
	_ = allowPartial
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_create requires at least one item"}
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-create")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchCreateOrdersRequest{
		RequestId: resolvedRequestID,
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	for _, item := range items {
		encoded, err := OrderIntentToProto(item, quantityScale)
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
		validated, err := requiredClientID(*item.ClientOrderID, "client_order_id")
		if err != nil {
			return nil, err
		}
		proto.ClientOrderId = validated
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
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-cancel")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchCancelOrdersRequest{
		RequestId: resolvedRequestID,
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
	if (item.NewPrice == nil || !item.NewPrice.IsSet()) && (item.NewQty == nil || !item.NewQty.IsSet()) {
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
		validated, err := requiredClientID(*item.ClientOrderID, "client_order_id")
		if err != nil {
			return nil, err
		}
		proto.Key = &orderv1.BatchModifyItem_ClientOrderId{ClientOrderId: validated}
	}
	if item.NewPrice != nil && item.NewPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*item.NewPrice, "new_price", "")
		if err != nil {
			return nil, err
		}
		proto.NewPriceTicks = &ticks
	}
	if item.NewQty != nil && item.NewQty.IsSet() {
		qty, err := ResolveQtyScaled(*item.NewQty, quantityScale, "new_qty", "", nil)
		if err != nil {
			return nil, err
		}
		proto.NewQtyScaled = &qty
	}
	if item.Behavior != nil {
		b, ok := modifyBehaviorToProto[strings.ToLower(*item.Behavior)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "behavior must be amend_or_replace, amend_only, or replace_only"}
		}
		proto.Behavior = b
	}
	validatedNewID, err := optionalClientID(item.NewClientOrderID, "new_client_order_id")
	if err != nil {
		return nil, err
	}
	if validatedNewID != nil {
		proto.NewClientOrderId = *validatedNewID
	}
	return proto, nil
}

// BatchModifyOrdersToProto encodes a batch modify request.
func BatchModifyOrdersToProto(items []models.BatchModifyItem, subAccountID *string, requestID *string, behaviorDefault *string, allowPartial bool, quantityScale int) (*orderv1.BatchModifyOrdersRequest, error) {
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_modify requires at least one item"}
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-mod")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchModifyOrdersRequest{
		RequestId:    resolvedRequestID,
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
