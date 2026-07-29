package codecs

import (
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
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
	if !item.Key.IsSet() {
		return nil, &errors.ValidationError{Msg: "each batch cancel item requires an OrderKey (OrderKeyByID or OrderKeyByClientID)"}
	}
	proto := &orderv1.BatchCancelItem{}
	if orderID, ok := item.Key.OrderID(); ok {
		id, err := IDToInt(orderID, "order_id")
		if err != nil {
			return nil, err
		}
		proto.OrderId = id
	} else {
		clientOrderID, _ := item.Key.ClientOrderID()
		validated, err := requiredClientID(clientOrderID, "client_order_id")
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

// BatchReplaceItemToProto encodes one batch replace item.
func BatchReplaceItemToProto(item models.BatchReplaceItem, quantityScale int) (*orderv1.BatchReplaceOrderItem, error) {
	if !item.Key.IsSet() {
		return nil, &errors.ValidationError{Msg: "each batch item requires an OrderKey (OrderKeyByID or OrderKeyByClientID)"}
	}
	if (item.NewPrice == nil || !item.NewPrice.IsSet()) && (item.NewQty == nil || !item.NewQty.IsSet()) {
		return nil, &errors.ValidationError{Msg: "each batch item requires new_price and/or new_qty"}
	}
	proto := &orderv1.BatchReplaceOrderItem{}
	if orderID, ok := item.Key.OrderID(); ok {
		id, err := IDToInt(orderID, "order_id")
		if err != nil {
			return nil, err
		}
		proto.Key = &orderv1.BatchReplaceOrderItem_OrderId{OrderId: id}
	} else {
		clientOrderID, _ := item.Key.ClientOrderID()
		validated, err := requiredClientID(clientOrderID, "client_order_id")
		if err != nil {
			return nil, err
		}
		proto.Key = &orderv1.BatchReplaceOrderItem_ClientOrderId{ClientOrderId: validated}
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
	validatedNewID, err := optionalClientID(item.NewClientOrderID, "new_client_order_id")
	if err != nil {
		return nil, err
	}
	if validatedNewID != nil {
		proto.NewClientOrderId = *validatedNewID
	}
	return proto, nil
}

// BatchReplaceOrdersToProto encodes a batch replace request.
func BatchReplaceOrdersToProto(items []models.BatchReplaceItem, symbolID uint32, subAccountID *string, requestID *string, quantityScale int) (*orderv1.BatchReplaceOrdersRequest, error) {
	if len(items) == 0 {
		return nil, &errors.ValidationError{Msg: "batch_replace requires at least one item"}
	}
	if symbolID == 0 {
		return nil, &errors.ValidationError{Msg: "batch_replace requires a resolved symbol_id"}
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-replace")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchReplaceOrdersRequest{
		SymbolId:  symbolID,
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
		encoded, err := BatchReplaceItemToProto(item, quantityScale)
		if err != nil {
			return nil, err
		}
		proto.Items = append(proto.Items, encoded)
	}
	return proto, nil
}
