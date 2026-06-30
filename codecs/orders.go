package codecs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var (
	orderSideToProto = map[string]orderv1.Side{
		"buy":  orderv1.Side_BUY,
		"sell": orderv1.Side_SELL,
	}
	orderTypeToProto = map[string]orderv1.OrderType{
		"limit":  orderv1.OrderType_LIMIT,
		"market": orderv1.OrderType_MARKET,
	}
	tifToProto = map[string]orderv1.TimeInForce{
		"gtc": orderv1.TimeInForce_GTC,
		"ioc": orderv1.TimeInForce_IOC,
		"fok": orderv1.TimeInForce_FOK,
	}
	modifyBehaviorToProto = map[string]orderv1.ModifyBehavior{
		"amend_or_replace": orderv1.ModifyBehavior_AMEND_OR_REPLACE,
		"amend_only":       orderv1.ModifyBehavior_AMEND_ONLY,
		"replace_only":     orderv1.ModifyBehavior_REPLACE_ONLY,
	}
)

// ParseOptionalSubaccountID parses optional subaccount id to uint64.
func ParseOptionalSubaccountID(value *string) (*uint64, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := IDToInt(*value, "sub_account_id")
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// QuantityScaleForSymbol returns quantity scale from catalog or default 8.
func QuantityScaleForSymbol(c *catalogs.Manager, symbol *string) int {
	if symbol != nil && c != nil {
		return c.BaseQuantityScaleForSymbol(*symbol)
	}
	return 8
}

// CreateOrderToProto encodes create order request.
func CreateOrderToProto(req models.CreateOrderRequest, quantityScale int) (*orderv1.CreateOrderRequest, error) {
	if req.Symbol == nil && req.SymbolID == nil {
		return nil, &errors.ValidationError{Msg: "orders.create requires symbol or symbol_id"}
	}
	side, ok := orderSideToProto[strings.ToLower(req.Side)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "side must be 'buy' or 'sell'"}
	}
	orderType, ok := orderTypeToProto[strings.ToLower(req.OrderType)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "order_type must be 'limit' or 'market'"}
	}
	qty, err := ParseQtyScaled(req.Qty, quantityScale, "qty")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.CreateOrderRequest{
		Side:      side,
		OrderType: orderType,
		QtyScaled: int64(qty),
	}
	if req.Symbol != nil {
		proto.Symbol = *req.Symbol
	}
	if req.Price != nil {
		ticks, err := ParsePriceTicks(*req.Price, "price")
		if err != nil {
			return nil, err
		}
		proto.PriceTicks = int64(ticks)
	}
	if req.TIF != nil {
		tif, ok := tifToProto[strings.ToLower(*req.TIF)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "tif must be one of 'gtc', 'ioc', or 'fok'"}
		}
		proto.TimeInForce = tif
	}
	if req.SubAccountID != nil {
		sub, err := IDToInt(*req.SubAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if req.ClientOrderID != nil {
		proto.ClientOrderId = *req.ClientOrderID
	}
	if req.PostOnly {
		proto.PostOnly = true
	}
	return proto, nil
}

// ModifyOrderToProto encodes modify order request.
func ModifyOrderToProto(
	symbol string,
	orderID *string,
	clientOrderID *string,
	subAccountID *string,
	requestID *string,
	newPrice, newQty *string,
	behavior *string,
	newClientOrderID *string,
	quantityScale int,
) (*orderv1.ModifyOrderRequest, error) {
	hasOrderID := orderID != nil && *orderID != ""
	hasClient := clientOrderID != nil && *clientOrderID != ""
	if hasOrderID == hasClient {
		return nil, &errors.ValidationError{Msg: "modify requires exactly one of order_id or client_order_id"}
	}
	if newPrice == nil && newQty == nil {
		return nil, &errors.ValidationError{Msg: "modify requires new_price, new_qty, and/or new_attached_risk"}
	}
	proto := &orderv1.ModifyOrderRequest{
		RequestId: coalesceRequestID(requestID, "mod"),
	}
	if hasOrderID {
		id, err := IDToInt(*orderID, "order_id")
		if err != nil {
			return nil, err
		}
		proto.Key = &orderv1.ModifyOrderRequest_OrderId{OrderId: id}
	}
	if hasClient {
		proto.Key = &orderv1.ModifyOrderRequest_ClientOrderId{ClientOrderId: *clientOrderID}
	}
	if subAccountID != nil {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if newPrice != nil {
		ticks, err := ParsePriceTicks(*newPrice, "new_price")
		if err != nil {
			return nil, err
		}
		v := int64(ticks)
		proto.NewPriceTicks = &v
	}
	if newQty != nil {
		qty, err := ParseQtyScaled(*newQty, quantityScale, "new_qty")
		if err != nil {
			return nil, err
		}
		v := int64(qty)
		proto.NewQtyScaled = &v
	}
	if behavior != nil {
		b, ok := modifyBehaviorToProto[strings.ToLower(*behavior)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "behavior must be amend_or_replace, amend_only, or replace_only"}
		}
		proto.Behavior = b
	}
	if newClientOrderID != nil {
		proto.NewClientOrderId = *newClientOrderID
	}
	return proto, nil
}

// CancelAllOrdersToProto encodes cancel-all request.
func CancelAllOrdersToProto(subAccountID *string, symbol, side *string, dryRun bool, maxOrders *int, requestID *string) (*orderv1.CancelAllOrdersRequest, error) {
	proto := &orderv1.CancelAllOrdersRequest{
		RequestId: coalesceRequestID(requestID, "cancel-all"),
		DryRun:    dryRun,
	}
	if subAccountID != nil {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if symbol != nil {
		proto.Symbol = *symbol
	}
	if side != nil {
		s, ok := orderSideToProto[strings.ToLower(*side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		proto.Side = s
	}
	if maxOrders != nil {
		v := uint32(*maxOrders)
		proto.MaxOrders = v
	}
	return proto, nil
}

// CancelAllAfterToProto encodes cancel-all-after request.
func CancelAllAfterToProto(subAccountID *string, timeoutSec int, symbol, side *string, requestID *string) (*orderv1.CancelAllAfterRequest, error) {
	proto := &orderv1.CancelAllAfterRequest{
		RequestId:  coalesceRequestID(requestID, "cancel-after"),
		TimeoutSec: uint32(timeoutSec),
	}
	if subAccountID != nil {
		sub, err := IDToInt(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if symbol != nil {
		proto.Symbol = *symbol
	}
	if side != nil {
		s, ok := orderSideToProto[strings.ToLower(*side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		proto.Side = s
	}
	return proto, nil
}

func coalesceRequestID(requestID *string, prefix string) string {
	if requestID != nil && *requestID != "" {
		return *requestID
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}
