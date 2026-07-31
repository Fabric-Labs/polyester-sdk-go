package codecs

import (
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
	feeAssetToProto = map[string]orderv1.FeeAsset{
		"quote": orderv1.FeeAsset_QUOTE,
		"base":  orderv1.FeeAsset_BASE,
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
// It does not silently fall back to scale 8 when catalogs or symbol are missing
// or unhydrated; write paths that encode decimal quantities must fail loudly.
func QuantityScaleForSymbol(c *catalogs.Manager, symbol *string) (int, error) {
	if c == nil || symbol == nil || *symbol == "" {
		return 0, &errors.ValidationError{Msg: "quantity scale requires catalogs and symbol"}
	}
	scale, ok := c.BaseQuantityScaleForSymbol(*symbol)
	if !ok {
		return 0, &errors.ValidationError{
			Msg: "quantity scale for " + *symbol + " is unavailable; call WaitForCatalogs before placing orders, or pass a scaled Quantity",
		}
	}
	return scale, nil
}

// QuoteQuantityScaleForSymbol returns quote quantity scale from the spot catalog.
//
// Quote-debit budgets must use this scale. It does not fall back to base scale.
func QuoteQuantityScaleForSymbol(c *catalogs.Manager, symbol *string) (int, error) {
	if c == nil || symbol == nil || *symbol == "" {
		return 0, &errors.ValidationError{Msg: "quote quantity scale requires catalogs and symbol"}
	}
	scale, ok := c.QuoteQuantityScaleForSymbol(*symbol)
	if !ok {
		return 0, &errors.ValidationError{
			Msg: "quote quantity scale for " + *symbol + " is unavailable; call WaitForCatalogs before placing quote-budget orders",
		}
	}
	return scale, nil
}

// OrderIntentToProto encodes the transport-independent OrderIntent shared by
// single and batch create. The flat public params (order_type/tif/post_only)
// are mapped onto the appropriate execution variant.
//
// quoteQuantityScale is required when MaxQuoteDebitScaled is set; otherwise it
// is ignored.
func OrderIntentToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.OrderIntent, error) {
	if req.Symbol == nil && req.SymbolID == nil {
		return nil, &errors.ValidationError{Msg: "orders.create requires symbol or symbol_id"}
	}
	if req.AttachedRisk != nil {
		return nil, &errors.ValidationError{Msg: "attached_risk is not supported by the current Go order input; refusing to discard it"}
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
	intent := &orderv1.OrderIntent{
		Symbol:   symbol,
		Side:     side,
		FeeAsset: orderv1.FeeAsset_QUOTE,
	}
	hasQty := req.Qty.IsSet()
	hasQuoteBudget := req.MaxQuoteDebitScaled.IsSet()
	if hasQty == hasQuoteBudget {
		return nil, &errors.ValidationError{Msg: "orders.create requires exactly one of qty or max_quote_debit_scaled"}
	}
	if hasQty {
		qty, err := ResolveQtyScaled(req.Qty, quantityScale, "qty", symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		intent.Sizing = &orderv1.OrderIntent_BaseQtyScaled{BaseQtyScaled: qty}
	} else {
		if side != orderv1.Side_BUY {
			return nil, &errors.ValidationError{Msg: "max_quote_debit_scaled is only valid for buy orders"}
		}
		budget, err := ResolveQuoteQtyScaled(req.MaxQuoteDebitScaled, quoteQuantityScale, "max_quote_debit_scaled", symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		intent.Sizing = &orderv1.OrderIntent_MaxQuoteDebitScaled{MaxQuoteDebitScaled: budget}
	}
	clientOrderID, err := optionalClientID(req.ClientOrderID, "client_order_id")
	if err != nil {
		return nil, err
	}
	if clientOrderID != nil {
		intent.ClientOrderId = *clientOrderID
	}
	if req.FeeAsset != nil {
		feeAsset, ok := feeAssetToProto[strings.ToLower(*req.FeeAsset)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "fee_asset must be quote or base"}
		}
		if feeAsset == orderv1.FeeAsset_BASE && side != orderv1.Side_BUY {
			return nil, &errors.ValidationError{Msg: "fee_asset=base is only valid for buy orders"}
		}
		intent.FeeAsset = feeAsset
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
		if hasPrice {
			return nil, &errors.ValidationError{
				Msg: "price is not valid for market orders; use market_client_ref_price for a reservation reference",
			}
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
	if hasQuoteBudget && orderType == "limit" && tif != "ioc" {
		return nil, &errors.ValidationError{Msg: "max_quote_debit_scaled is only valid for buy market or limit IOC orders"}
	}
	return intent, nil
}

// PreviewOrderToProto encodes the preview request from the same public input
// shape as create, preserving sizing, execution, and fee-asset semantics.
func PreviewOrderToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.PreviewOrderRequest, error) {
	intent, err := OrderIntentToProto(req, quantityScale, quoteQuantityScale)
	if err != nil {
		return nil, err
	}
	proto := &orderv1.PreviewOrderRequest{
		Symbol:   intent.GetSymbol(),
		Side:     intent.GetSide(),
		FeeAsset: intent.GetFeeAsset(),
	}
	switch sizing := intent.GetSizing().(type) {
	case *orderv1.OrderIntent_BaseQtyScaled:
		proto.Sizing = &orderv1.PreviewOrderRequest_BaseQtyScaled{BaseQtyScaled: sizing.BaseQtyScaled}
	case *orderv1.OrderIntent_MaxQuoteDebitScaled:
		proto.Sizing = &orderv1.PreviewOrderRequest_MaxQuoteDebitScaled{MaxQuoteDebitScaled: sizing.MaxQuoteDebitScaled}
	default:
		return nil, &errors.ValidationError{Msg: "orders.preview requires sizing"}
	}
	switch execution := intent.GetExecution().(type) {
	case *orderv1.OrderIntent_MarketIoc:
		proto.Execution = &orderv1.PreviewOrderRequest_MarketIoc{MarketIoc: execution.MarketIoc}
	case *orderv1.OrderIntent_LimitGtc:
		proto.Execution = &orderv1.PreviewOrderRequest_LimitGtc{LimitGtc: execution.LimitGtc}
	case *orderv1.OrderIntent_LimitIoc:
		proto.Execution = &orderv1.PreviewOrderRequest_LimitIoc{LimitIoc: execution.LimitIoc}
	case *orderv1.OrderIntent_LimitFok:
		proto.Execution = &orderv1.PreviewOrderRequest_LimitFok{LimitFok: execution.LimitFok}
	default:
		return nil, &errors.ValidationError{Msg: "orders.preview requires execution"}
	}
	if req.SubAccountID != nil {
		sub, err := IDToInt(*req.SubAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	return proto, nil
}

// CreateOrderToProto encodes create order request.
func CreateOrderToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.CreateOrderRequest, error) {
	intent, err := OrderIntentToProto(req, quantityScale, quoteQuantityScale)
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
	key models.OrderKey,
	subAccountID *string,
	requestID *string,
	newPrice *models.PriceInput,
	newQty *models.QtyInput,
	behavior *string,
	newClientOrderID *string,
	quantityScale int,
) (*orderv1.ModifyOrderRequest, error) {
	if (newPrice == nil || !newPrice.IsSet()) && (newQty == nil || !newQty.IsSet()) {
		return nil, &errors.ValidationError{Msg: "modify requires new_price, new_qty, and/or new_attached_risk"}
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "mod")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.ModifyOrderRequest{
		RequestId: resolvedRequestID,
	}
	if err := ApplyOrderKeyToModify(proto, key); err != nil {
		return nil, err
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
	validatedNewID, err := optionalClientID(newClientOrderID, "new_client_order_id")
	if err != nil {
		return nil, err
	}
	if validatedNewID != nil {
		proto.NewClientOrderId = *validatedNewID
	}
	return proto, nil
}

// CancelAllOrdersToProto encodes cancel-all request.
func CancelAllOrdersToProto(subAccountID *string, symbol, side *string, dryRun bool, requestID *string) (*orderv1.CancelAllOrdersRequest, error) {
	resolvedRequestID, err := coalesceRequestID(requestID, "cancel-all")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.CancelAllOrdersRequest{
		RequestId: resolvedRequestID,
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
	resolvedRequestID, err := coalesceRequestID(requestID, "cancel-after")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.CancelAllAfterRequest{
		RequestId:  resolvedRequestID,
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
