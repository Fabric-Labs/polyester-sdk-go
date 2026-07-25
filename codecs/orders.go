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

// QuantityScaleForSymbol returns quantity scale from the spot catalog.
//
// It does not silently fall back to scale 8 when catalogs or symbol are missing;
// write paths that encode decimal quantities must fail loudly instead.
func QuantityScaleForSymbol(c *catalogs.Manager, symbol *string) (int, error) {
	if c == nil || symbol == nil || *symbol == "" {
		return 0, &errors.ValidationError{Msg: "quantity scale requires catalogs and symbol"}
	}
	return c.BaseQuantityScaleForSymbol(*symbol), nil
}

// OrderIntentToProto encodes the transport-independent OrderIntent shared by
// single and batch create. The flat public params (order_type/tif/post_only)
// are mapped onto the appropriate execution variant.
func OrderIntentToProto(req models.CreateOrderRequest, quantityScale int) (*orderv1.OrderIntent, error) {
	if req.Symbol == nil && req.SymbolID == nil {
		return nil, &errors.ValidationError{Msg: "orders.create requires symbol or symbol_id"}
	}
	side, ok := orderSideToProto[strings.ToLower(req.Side)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "side must be 'buy' or 'sell'"}
	}
	orderType := strings.ToLower(req.OrderType)
	if orderType != "limit" && orderType != "market" {
		return nil, &errors.ValidationError{Msg: "order_type must be 'limit' or 'market'"}
	}
	symbol := ""
	if req.Symbol != nil {
		symbol = *req.Symbol
	}
	qty, err := ResolveQtyScaled(req.Qty, quantityScale, "qty", symbol, req.SymbolID)
	if err != nil {
		return nil, err
	}
	intent := &orderv1.OrderIntent{
		Symbol:    symbol,
		Side:      side,
		QtyScaled: qty,
	}
	if req.ClientOrderID != nil {
		intent.ClientOrderId = *req.ClientOrderID
	}

	hasPrice := req.Price != nil && req.Price.IsSet()
	var priceTicks int64
	if hasPrice {
		ticks, err := ResolvePriceTicks(*req.Price, "price", symbol)
		if err != nil {
			return nil, err
		}
		priceTicks = ticks
	}
	tif := ""
	if req.TIF != nil {
		tif = strings.ToLower(*req.TIF)
		if tif != "gtc" && tif != "ioc" && tif != "fok" {
			return nil, &errors.ValidationError{Msg: "tif must be one of 'gtc', 'ioc', or 'fok'"}
		}
	}

	switch orderType {
	case "market":
		if req.PostOnly {
			return nil, &errors.ValidationError{Msg: "post_only is not supported for market orders"}
		}
		market := &orderv1.MarketIoc{}
		if req.MarketClientRefPrice != nil && req.MarketClientRefPrice.IsSet() {
			ticks, err := ResolvePriceTicks(*req.MarketClientRefPrice, "market_client_ref_price", symbol)
			if err != nil {
				return nil, err
			}
			market.ClientRefPriceTicks = ticks
		}
		intent.Execution = &orderv1.OrderIntent_MarketIoc{MarketIoc: market}
	case "limit":
		if !hasPrice {
			return nil, &errors.ValidationError{Msg: "limit orders require price"}
		}
		switch tif {
		case "ioc":
			if req.PostOnly {
				return nil, &errors.ValidationError{Msg: "post_only is not supported for ioc limit orders"}
			}
			intent.Execution = &orderv1.OrderIntent_LimitIoc{LimitIoc: &orderv1.LimitIoc{PriceTicks: priceTicks}}
		case "fok":
			if req.PostOnly {
				return nil, &errors.ValidationError{Msg: "post_only is not supported for fok limit orders"}
			}
			intent.Execution = &orderv1.OrderIntent_LimitFok{LimitFok: &orderv1.LimitFok{PriceTicks: priceTicks}}
		default: // gtc or unspecified
			intent.Execution = &orderv1.OrderIntent_LimitGtc{LimitGtc: &orderv1.LimitGtc{PriceTicks: priceTicks, PostOnly: req.PostOnly}}
		}
	}
	return intent, nil
}

// CreateOrderToProto encodes create order request.
func CreateOrderToProto(req models.CreateOrderRequest, quantityScale int) (*orderv1.CreateOrderRequest, error) {
	intent, err := OrderIntentToProto(req, quantityScale)
	if err != nil {
		return nil, err
	}
	proto := &orderv1.CreateOrderRequest{Order: intent}
	if req.SubAccountID != nil {
		sub, err := IDToInt(*req.SubAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
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
	newPrice *models.PriceInput,
	newQty *models.QtyInput,
	behavior *string,
	newClientOrderID *string,
	quantityScale int,
) (*orderv1.ModifyOrderRequest, error) {
	hasOrderID := orderID != nil && *orderID != ""
	hasClient := clientOrderID != nil && *clientOrderID != ""
	if hasOrderID == hasClient {
		return nil, &errors.ValidationError{Msg: "modify requires exactly one of order_id or client_order_id"}
	}
	if (newPrice == nil || !newPrice.IsSet()) && (newQty == nil || !newQty.IsSet()) {
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
	if newPrice != nil && newPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*newPrice, "new_price", symbol)
		if err != nil {
			return nil, err
		}
		proto.NewPriceTicks = &ticks
	}
	if newQty != nil && newQty.IsSet() {
		qty, err := ResolveQtyScaled(*newQty, quantityScale, "new_qty", symbol, nil)
		if err != nil {
			return nil, err
		}
		proto.NewQtyScaled = &qty
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
func CancelAllOrdersToProto(subAccountID *string, symbol, side *string, dryRun bool, requestID *string) (*orderv1.CancelAllOrdersRequest, error) {
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
