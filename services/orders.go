package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// OrdersService wraps order read/write RPCs.
type OrdersService struct {
	transport            *transport.Factory
	catalogs             *catalogs.Manager
	scoped               ScopedSubAccount
	defaultAccountID     *string
	realtime             RealtimeClient
	catalogHydrationDone <-chan struct{}
	catalogLastError     func() error
}

// NewOrdersService constructs OrdersService.
func NewOrdersService(
	factory *transport.Factory,
	cats *catalogs.Manager,
	defaultSubAccountID *string,
	realtime RealtimeClient,
	defaultAccountID *string,
	catalogHydrationDone <-chan struct{},
	catalogLastError func() error,
) *OrdersService {
	return &OrdersService{
		transport:            factory,
		catalogs:             cats,
		scoped:               ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID},
		realtime:             realtime,
		defaultAccountID:     defaultAccountID,
		catalogHydrationDone: catalogHydrationDone,
		catalogLastError:     catalogLastError,
	}
}

func (s *OrdersService) ensureCatalogs(ctx context.Context) error {
	if s.catalogHydrationDone == nil {
		return nil
	}
	select {
	case <-s.catalogHydrationDone:
		if s.catalogLastError != nil {
			return s.catalogLastError()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *OrdersService) readClient() ordersv1connect.OrdersReadServiceClient {
	return ordersv1connect.NewOrdersReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *OrdersService) writeClient() ordersv1connect.OrdersServiceClient {
	return ordersv1connect.NewOrdersServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func boolPtr(v bool) *bool { return &v }

func uint32Ptr(v uint32) *uint32 { return &v }

// ListOpen returns open orders.
// When triggerID is set, only child orders created by that trigger are returned.
func (s *OrdersService) ListOpen(ctx context.Context, account AccountScope, subAccountID *string, pageToken *string, limit *int, includeAttachedRisk, includeAttachedRiskState bool, triggerID *string) (models.OrdersList, error) {
	req := &orderv1.GetOpenOrdersRequest{
		IncludeAttachedRisk: boolPtr(includeAttachedRisk), IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.OrdersList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	parsedLimit, err := ExplicitPaginationLimit(limit, "limit")
	if err != nil {
		return models.OrdersList{}, err
	}
	if parsedLimit != nil {
		req.Limit = parsedLimit
	}
	if triggerID != nil && *triggerID != "" {
		id, err := codecs.IDToInt(*triggerID, "trigger_id")
		if err != nil {
			return models.OrdersList{}, err
		}
		req.TriggerId = &id
	}
	return UnaryAuthDecoded(ctx, s.transport, s.readClient().GetOpenOrders, req, decode.OrdersListFromOpenChecked)
}

// ListHistory returns order history.
// When triggerID is set, only child orders created by that trigger are returned.
func (s *OrdersService) ListHistory(ctx context.Context, account AccountScope, subAccountID, symbol *string, symbolID *uint32, pageToken *string, limit int, includeAttachedRisk, includeAttachedRiskState bool, triggerID *string) (models.OrdersList, error) {
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.OrdersList{}, err
	}
	req := &orderv1.GetOrderHistoryRequest{
		Limit: uint32Ptr(parsedLimit), IncludeAttachedRisk: boolPtr(includeAttachedRisk),
		IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.OrdersList{}, err
	}
	if symbolID != nil {
		req.SymbolId = append(req.SymbolId, *symbolID)
	} else if symbol != nil {
		resolved, err := ResolveSymbolID(s.catalogs, symbol, nil, "list_history")
		if err != nil {
			return models.OrdersList{}, err
		}
		req.SymbolId = append(req.SymbolId, resolved)
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	if triggerID != nil && *triggerID != "" {
		id, err := codecs.IDToInt(*triggerID, "trigger_id")
		if err != nil {
			return models.OrdersList{}, err
		}
		req.TriggerId = &id
	}
	return UnaryAuthDecoded(ctx, s.transport, s.readClient().GetOrderHistory, req, decode.OrdersListFromHistoryChecked)
}

// Get returns one order.
func (s *OrdersService) Get(ctx context.Context, account AccountScope, key models.OrderKey, subAccountID *string, includeAttachedRisk, includeAttachedRiskState bool) (models.GetOrderResult, error) {
	req := &orderv1.GetOrderRequest{
		IncludeAttachedRisk: boolPtr(includeAttachedRisk), IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.GetOrderResult{}, err
	}
	if err := codecs.ApplyOrderKeyToGet(req, key); err != nil {
		return models.GetOrderResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.readClient().GetOrder, req, decode.GetOrderFromProtoChecked)
}

// Create places a new order.
func (s *OrdersService) Create(ctx context.Context, req models.CreateOrderRequest, account AccountScope) (models.OrderMutationResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.OrderMutationResult{}, err
	}
	if account != nil {
		sub, err := s.scoped.ResolveSubAccountID(nil, account)
		if err != nil {
			return models.OrderMutationResult{}, err
		}
		req.SubAccountID = sub
	}
	if req.SubAccountID == nil {
		sub, err := s.scoped.ResolveSubAccountID(nil, nil)
		if err != nil {
			return models.OrderMutationResult{}, err
		}
		req.SubAccountID = sub
	}
	scale, err := quantityScaleForOrderWrite(s.catalogs, req.Symbol, req.SymbolID)
	if err != nil {
		return models.OrderMutationResult{}, err
	}
	quoteScale, err := quoteQuantityScaleForOrderWrite(s.catalogs, req.Symbol, req.SymbolID, req.MaxQuoteDebitScaled.IsSet())
	if err != nil {
		return models.OrderMutationResult{}, err
	}
	protoReq, err := codecs.CreateOrderToProto(req, scale, quoteScale)
	if err != nil {
		return models.OrderMutationResult{}, err
	}
	if constraints, ok := pairConstraints(s.catalogs, req.Symbol, req.SymbolID); ok {
		if err := preflightOrderIntent(constraints, protoReq.GetOrder()); err != nil {
			return models.OrderMutationResult{}, err
		}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CreateOrder, protoReq, decode.OrderMutationFromProto)
}

// Preview checks whether an order intent is currently admissible without
// creating it. The result may include resolved base quantity and a protected
// price bound when sizing/protection were evaluated. It is advisory: account,
// market, and policy inputs may change, and CreateOrder always re-evaluates.
// Preview is not deployed on every API host; treat unimplemented/not-found as
// non-fatal and do not make Preview a prerequisite for order submission.
func (s *OrdersService) Preview(ctx context.Context, req models.CreateOrderRequest, account AccountScope) (models.PreviewOrderResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.PreviewOrderResult{}, err
	}
	if account != nil {
		sub, err := s.scoped.ResolveSubAccountID(nil, account)
		if err != nil {
			return models.PreviewOrderResult{}, err
		}
		req.SubAccountID = sub
	}
	if req.SubAccountID == nil {
		sub, err := s.scoped.ResolveSubAccountID(nil, nil)
		if err != nil {
			return models.PreviewOrderResult{}, err
		}
		req.SubAccountID = sub
	}
	baseScale, err := quantityScaleForOrderWrite(s.catalogs, req.Symbol, req.SymbolID)
	if err != nil {
		return models.PreviewOrderResult{}, err
	}
	// Quote scale is still required to encode BUY max_quote_debit on the request.
	quoteScale, err := quoteQuantityScaleForOrderWrite(s.catalogs, req.Symbol, req.SymbolID, true)
	if err != nil {
		return models.PreviewOrderResult{}, err
	}
	protoReq, err := codecs.PreviewOrderToProto(req, baseScale, quoteScale)
	if err != nil {
		return models.PreviewOrderResult{}, err
	}
	if constraints, ok := pairConstraints(s.catalogs, req.Symbol, req.SymbolID); ok {
		if err := preflightOrderIntent(constraints, protoReq.GetOrder()); err != nil {
			return models.PreviewOrderResult{}, err
		}
	}
	symbol := ""
	if req.Symbol != nil {
		symbol = *req.Symbol
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().PreviewOrder, protoReq, func(msg *orderv1.PreviewOrderResponse) (models.PreviewOrderResult, error) {
		return decode.PreviewOrderFromProto(msg, baseScale, symbol, req.SymbolID)
	})
}

// Cancel cancels an order.
func (s *OrdersService) Cancel(ctx context.Context, account AccountScope, key models.OrderKey, symbol *string, symbolID *uint32, subAccountID *string) (models.OrderMutationResult, error) {
	if symbol != nil && symbolID != nil {
		return models.OrderMutationResult{}, &sdkerrors.ValidationError{Msg: "cancel accepts symbol or symbol_id, not both"}
	}
	if symbolID != nil && *symbolID == 0 {
		return models.OrderMutationResult{}, &sdkerrors.ValidationError{Msg: "symbol_id must be non-zero when explicitly supplied"}
	}
	req := &orderv1.CancelOrderRequest{}
	if err := codecs.ApplyOrderKeyToCancel(req, key); err != nil {
		return models.OrderMutationResult{}, err
	}
	if symbolID != nil {
		req.SymbolId = *symbolID
	} else if symbol != nil {
		id, err := ResolveSymbolID(s.catalogs, symbol, nil, "cancel")
		if err != nil {
			return models.OrderMutationResult{}, err
		}
		req.SymbolId = id
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.OrderMutationResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CancelOrder, req, decode.OrderMutationFromCancel)
}

// Modify modifies an open order.
func (s *OrdersService) Modify(ctx context.Context, account AccountScope, symbol string, key models.OrderKey, subAccountID, requestID *string, newPrice *models.PriceInput, newQty *models.QtyInput, behavior, newClientOrderID *string) (models.ModifyOrderResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.ModifyOrderResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &symbol)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	protoReq, err := codecs.ModifyOrderToProto(symbol, key, sub, requestID, newPrice, newQty, behavior, newClientOrderID, scale)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	if constraints, ok := pairConstraints(s.catalogs, &symbol, nil); ok {
		var prices []int64
		var notionalPrice *int64
		if protoReq.NewPriceTicks != nil {
			prices = append(prices, protoReq.GetNewPriceTicks())
			if protoReq.NewQtyScaled != nil {
				value := protoReq.GetNewPriceTicks()
				notionalPrice = &value
			}
		}
		if err := preflightPairValues(constraints, protoReq.GetNewQtyScaled(), prices, notionalPrice); err != nil {
			return models.ModifyOrderResult{}, err
		}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().ModifyOrder, protoReq, decode.ModifyOrderFromProto)
}

// CancelAll cancels all matching orders.
func (s *OrdersService) CancelAll(ctx context.Context, account AccountScope, subAccountID, symbol, side *string, dryRun bool, requestID *string) (models.CancelAllOrdersResult, error) {
	if err := ValidateSymbolFilter(s.catalogs, symbol, "cancel_all"); err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	protoReq, err := codecs.CancelAllOrdersToProto(sub, symbol, side, dryRun, requestID)
	if err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CancelAllOrders, protoReq, decode.CancelAllFromProto)
}

// CancelAllAfter arms cancel-all-after.
func (s *OrdersService) CancelAllAfter(ctx context.Context, account AccountScope, timeoutSec int, subAccountID, symbol, side, requestID *string) (models.CancelAllAfterResult, error) {
	if err := ValidateSymbolFilter(s.catalogs, symbol, "cancel_all_after"); err != nil {
		return models.CancelAllAfterResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.CancelAllAfterResult{}, err
	}
	protoReq, err := codecs.CancelAllAfterToProto(sub, timeoutSec, symbol, side, requestID)
	if err != nil {
		return models.CancelAllAfterResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CancelAllAfter, protoReq, decode.CancelAllAfterFromProto)
}

// BatchCreate places multiple orders in one request.
func (s *OrdersService) BatchCreate(ctx context.Context, account AccountScope, items []models.CreateOrderRequest, subAccountID *string, symbol *string, requestID *string, allowPartial bool) (models.BatchCreateOrdersResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.BatchCreateOrdersResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BatchCreateOrdersResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, symbol)
	if err != nil {
		return models.BatchCreateOrdersResult{}, err
	}
	needsQuoteScale := false
	for _, item := range items {
		if item.MaxQuoteDebitScaled.IsSet() {
			needsQuoteScale = true
			break
		}
	}
	quoteScale, err := quoteQuantityScaleForOrderWrite(s.catalogs, symbol, nil, needsQuoteScale)
	if err != nil {
		return models.BatchCreateOrdersResult{}, err
	}
	protoReq, err := codecs.BatchCreateOrdersToProto(items, sub, requestID, allowPartial, scale, quoteScale)
	if err != nil {
		return models.BatchCreateOrdersResult{}, err
	}
	if constraints, ok := pairConstraints(s.catalogs, symbol, nil); ok {
		for _, intent := range protoReq.GetItems() {
			if err := preflightOrderIntent(constraints, intent); err != nil {
				return models.BatchCreateOrdersResult{}, err
			}
		}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().BatchCreateOrders, protoReq, decode.BatchCreateFromProto)
}

// BatchCancel cancels multiple orders in one request.
func (s *OrdersService) BatchCancel(ctx context.Context, account AccountScope, items []models.BatchCancelItem, subAccountID *string, requestID *string) (models.BatchCancelOrdersResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BatchCancelOrdersResult{}, err
	}
	protoReq, err := codecs.BatchCancelOrdersToProto(items, sub, requestID)
	if err != nil {
		return models.BatchCancelOrdersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().BatchCancelOrders, protoReq, decode.BatchCancelFromProto)
}

// BatchReplace replaces multiple same-symbol orders and returns after admission.
//
// The response is an admission receipt. Old IDs are the stale predecessors;
// replacement IDs are successors. Poll GetBatchReplaceStatus to reconcile item
// phases and IDs, not to infer execution finality. Reuse the same requestID
// when retrying the same logical batch.
func (s *OrdersService) BatchReplace(ctx context.Context, account AccountScope, items []models.BatchReplaceItem, symbol string, subAccountID *string, requestID *string) (models.BatchReplaceOrdersResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	symbolID, err := ResolveSymbolID(s.catalogs, &symbol, nil, "batch_replace")
	if err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &symbol)
	if err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	protoReq, err := codecs.BatchReplaceOrdersToProto(items, symbolID, sub, requestID, scale)
	if err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	if constraints, ok := pairConstraints(s.catalogs, &symbol, nil); ok {
		for _, item := range protoReq.GetItems() {
			var prices []int64
			var notionalPrice *int64
			if item.NewPriceTicks != nil {
				prices = append(prices, item.GetNewPriceTicks())
				if item.NewQtyScaled != nil {
					value := item.GetNewPriceTicks()
					notionalPrice = &value
				}
			}
			if err := preflightPairValues(constraints, item.GetNewQtyScaled(), prices, notionalPrice); err != nil {
				return models.BatchReplaceOrdersResult{}, err
			}
		}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().BatchReplaceOrders, protoReq, decode.BatchReplaceFromProto)
}

// GetBatchReplaceStatus returns durable reconciliation status for one admitted
// batch. A working successor is live, but not necessarily execution-final.
func (s *OrdersService) GetBatchReplaceStatus(ctx context.Context, account AccountScope, batchRequestID string, subAccountID *string) (models.BatchReplaceStatusResult, error) {
	id, err := codecs.IDToInt(batchRequestID, "batch_request_id")
	if err != nil {
		return models.BatchReplaceStatusResult{}, err
	}
	req := &orderv1.GetBatchReplaceStatusRequest{BatchRequestId: id}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.BatchReplaceStatusResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.readClient().GetBatchReplaceStatus, req, decode.BatchReplaceStatusFromProto)
}

// Subscribe streams private order updates for an account.
func (s *OrdersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.Order], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:spot:orders:{account_id}:proto", accountID, s.defaultAccountID, decode.OrderFromBytes)
}

// WaitForOrderTradesComplete polls GetOrder until the sum of returned trade
// quantities equals the order's cumulative filled quantity, or the timeout elapses.
//
// Trade projection can lag the order filled/cum qty fields (POLY-3750); use this
// helper when fills must be fully projected before continuing.
func (s *OrdersService) WaitForOrderTradesComplete(
	ctx context.Context,
	account AccountScope,
	key models.OrderKey,
	subAccountID *string,
	timeout time.Duration,
) (models.GetOrderResult, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var last models.GetOrderResult
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		detail, err := s.Get(ctx, account, key, subAccountID, false, false)
		if err != nil {
			return last, err
		}
		last = detail
		if detail.Order != nil && orderTradesComplete(detail) {
			return detail, nil
		}
		if time.Now().After(deadline) {
			cum := int64(0)
			if detail.Order != nil {
				cum = detail.Order.CumQty.Scaled()
			}
			return last, fmt.Errorf(
				"order trades not complete within %s: cum_qty=%d trade_qty_sum=%d trades=%d",
				timeout, cum, sumTradeQty(detail.Trades), len(detail.Trades),
			)
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func orderTradesComplete(detail models.GetOrderResult) bool {
	if detail.Order == nil {
		return false
	}
	return sumTradeQty(detail.Trades) == detail.Order.CumQty.Scaled()
}

func sumTradeQty(trades []models.UserTrade) int64 {
	var sum int64
	for _, trade := range trades {
		sum += trade.Qty.Scaled()
	}
	return sum
}

func quantityScaleForOrderWrite(c *catalogs.Manager, symbol *string, symbolID *uint32) (int, error) {
	if symbol != nil && *symbol != "" {
		return codecs.QuantityScaleForSymbol(c, symbol)
	}
	if c != nil && symbolID != nil {
		scale, ok := c.BaseQuantityScaleForSymbolID(*symbolID)
		if !ok {
			return 0, &sdkerrors.ValidationError{
				Msg: "quantity scale for symbol_id is unavailable; call WaitForCatalogs before placing orders, or pass a scaled Quantity",
			}
		}
		return scale, nil
	}
	return 0, &sdkerrors.ValidationError{Msg: "quantity scale requires catalogs and symbol"}
}

func quoteQuantityScaleForOrderWrite(c *catalogs.Manager, symbol *string, symbolID *uint32, required bool) (int, error) {
	if !required {
		return 0, nil
	}
	if symbol != nil && *symbol != "" {
		return codecs.QuoteQuantityScaleForSymbol(c, symbol)
	}
	if c != nil && symbolID != nil {
		scale, ok := c.QuoteQuantityScaleForSymbolID(*symbolID)
		if !ok {
			return 0, &sdkerrors.ValidationError{
				Msg: "quote quantity scale for symbol_id is unavailable; call WaitForCatalogs before placing quote-budget orders",
			}
		}
		return scale, nil
	}
	return 0, &sdkerrors.ValidationError{Msg: "quote quantity scale requires catalogs and symbol"}
}
