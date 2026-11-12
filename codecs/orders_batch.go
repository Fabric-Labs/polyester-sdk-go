package codecs

import (
	"fmt"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Item limits of each batch operation, read from the public contract's
// validation rule on the request's items field: 20 for BatchCreate, 50 for
// BatchCancel and BatchReplace. A larger batch is refused before sending.
var (
	MaxBatchCreateItems  = contractMaxItems(&orderv1.BatchCreateOrdersRequest{})
	MaxBatchCancelItems  = contractMaxItems(&orderv1.BatchCancelOrdersRequest{})
	MaxBatchReplaceItems = contractMaxItems(&orderv1.BatchReplaceOrdersRequest{})
)

func contractMaxItems(request proto.Message) int {
	return contractMaxRepeated(request, "items")
}

// contractMaxRepeated reads the contract's max_items rule of a repeated
// request field.
func contractMaxRepeated(request proto.Message, name protoreflect.Name) int {
	field := request.ProtoReflect().Descriptor().Fields().ByName(name)
	rules, _ := proto.GetExtension(field.Options(), validate.E_Field).(*validate.FieldRules)
	return int(rules.GetRepeated().GetMaxItems())
}

func validateBatchSize(operation string, lenItems, maxItems int) error {
	if lenItems == 0 {
		return &errors.ValidationError{Msg: operation + " requires at least one item"}
	}
	if lenItems > maxItems {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("%s accepts at most %d items", operation, maxItems),
		}
	}
	return nil
}

// OrderScales holds the catalog scales of one batch item's destination pair.
type OrderScales struct {
	Base  int
	Quote int
}

// BatchCreateOrdersToProto encodes a batch create request.
//
// The allowPartial argument is retained for API compatibility but is ignored:
// the POLY-3701 wire contract removed the batch allow_partial field.
func BatchCreateOrdersToProto(items []models.CreateOrderRequest, subAccountID *string, requestID *string, allowPartial bool, quantityScale, quoteQuantityScale int, itemScales ...OrderScales) (*orderv1.BatchCreateOrdersRequest, error) {
	_ = allowPartial
	if err := validateBatchSize("batch_create", len(items), MaxBatchCreateItems); err != nil {
		return nil, err
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-create")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchCreateOrdersRequest{
		RequestId: resolvedRequestID,
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if len(itemScales) != 0 && len(itemScales) != len(items) {
		return nil, &errors.ValidationError{Msg: "batch_create requires scales for every item"}
	}
	seenClientOrderIDs := make(map[string]struct{}, len(items))
	for i, item := range items {
		if item.SubAccountID != nil {
			return nil, &errors.ValidationError{Msg: "batch_create item sub_account_id is unsupported; use the batch-level subAccountID"}
		}
		if len(itemScales) != 0 {
			quantityScale, quoteQuantityScale = itemScales[i].Base, itemScales[i].Quote
		}
		encoded, err := OrderIntentToProto(item, quantityScale, quoteQuantityScale)
		if err != nil {
			return nil, err
		}
		if encoded.ClientOrderId != "" {
			if _, duplicate := seenClientOrderIDs[encoded.ClientOrderId]; duplicate {
				return nil, &errors.ValidationError{
					Msg: fmt.Sprintf("batch_create contains duplicate client_order_id %q", encoded.ClientOrderId),
				}
			}
			seenClientOrderIDs[encoded.ClientOrderId] = struct{}{}
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
	if err := validateBatchSize("batch_cancel", len(items), MaxBatchCancelItems); err != nil {
		return nil, err
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "batch-cancel")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.BatchCancelOrdersRequest{
		RequestId: resolvedRequestID,
	}
	if subAccountID != nil && *subAccountID != "" {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
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

// OrderIdentity supplies the destination pair for typed money compatibility checks.
type OrderIdentity struct {
	Symbol   string
	SymbolID *uint32
}

// BatchReplaceItemToProto encodes one batch replace item.
func BatchReplaceItemToProto(item models.BatchReplaceItem, quantityScale int, destination ...OrderIdentity) (*orderv1.BatchReplaceOrderItem, error) {
	identity := OrderIdentity{}
	if len(destination) > 0 {
		identity = destination[0]
	}
	if !item.Key.IsSet() {
		return nil, &errors.ValidationError{Msg: "each batch item requires an OrderKey (OrderKeyByID or OrderKeyByClientID)"}
	}
	if (item.NewPrice == nil || !item.NewPrice.IsSet()) && (item.NewQty == nil || !item.NewQty.IsSet()) && item.NewAttachedRisk == nil {
		return nil, &errors.ValidationError{Msg: "each batch item requires new_price, new_qty, and/or new_attached_risk"}
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
		ticks, err := ResolvePriceTicks(*item.NewPrice, "new_price", identity.Symbol)
		if err != nil {
			return nil, err
		}
		proto.NewPriceTicks = &ticks
	}
	if item.NewQty != nil && item.NewQty.IsSet() {
		qty, err := ResolveQtyScaled(*item.NewQty, quantityScale, "new_qty", identity.Symbol, identity.SymbolID)
		if err != nil {
			return nil, err
		}
		proto.NewQtyScaled = &qty
	}
	risk, err := RiskPolicyToProto(item.NewAttachedRisk, "new_attached_risk", identity.Symbol)
	if err != nil {
		return nil, err
	}
	proto.NewAttachedRisk = risk
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
func BatchReplaceOrdersToProto(items []models.BatchReplaceItem, symbolID uint32, subAccountID *string, requestID *string, quantityScale int, symbol ...string) (*orderv1.BatchReplaceOrdersRequest, error) {
	if err := validateBatchSize("batch_replace", len(items), MaxBatchReplaceItems); err != nil {
		return nil, err
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
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	for _, item := range items {
		identity := OrderIdentity{SymbolID: &symbolID}
		if len(symbol) > 0 {
			identity.Symbol = symbol[0]
		}
		encoded, err := BatchReplaceItemToProto(item, quantityScale, identity)
		if err != nil {
			return nil, err
		}
		proto.Items = append(proto.Items, encoded)
	}
	return proto, nil
}
