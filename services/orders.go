package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
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
		return awaitCatalogs(ctx, s.catalogs, true)
	}
	select {
	case <-s.catalogHydrationDone:
		if s.catalogLastError != nil {
			return hydrationFailure(s.catalogLastError())
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// resolveSelectors resolves a market given by name, by symbol_id or by both
// through ResolveSymbolID, which refuses a name and an id of different
// markets. A name waits for the catalogs it is resolved or checked against.
func (s *OrdersService) resolveSelectors(ctx context.Context, symbol *string, symbolID *uint32, label string) (uint32, error) {
	if namesSymbol(symbol) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return 0, err
		}
	}
	return ResolveSymbolID(s.catalogs, symbol, symbolID, label)
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
	scales, err := readQuantityScales(ctx, s.catalogs)
	if err != nil {
		return models.OrdersList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.readClient().GetOpenOrders, req, scaledRead(decode.OrdersListFromOpen, scaleOrdersList, scales))
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
	if symbol != nil || symbolID != nil {
		resolved, err := s.resolveSelectors(ctx, symbol, symbolID, "list_history")
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
	scales, err := readQuantityScales(ctx, s.catalogs)
	if err != nil {
		return models.OrdersList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.readClient().GetOrderHistory, req, scaledRead(decode.OrdersListFromHistory, scaleOrdersList, scales))
}

// Get returns one order and an optional page of lineage execution history.
// Execution history defaults to enabled on the server. Pass
// IncludeExecutionHistory=false for state-only polling and omit Limit/PageToken.
func (s *OrdersService) Get(ctx context.Context, account AccountScope, key models.OrderKey, subAccountID *string, includeAttachedRisk, includeAttachedRiskState bool, opts ...models.GetOrderOptions) (models.GetOrderResult, error) {
	var options models.GetOrderOptions
	if len(opts) > 1 {
		return models.GetOrderResult{}, &sdkerrors.ValidationError{Msg: "orders.Get accepts at most one GetOrderOptions value"}
	}
	if len(opts) == 1 {
		options = opts[0]
	}
	if options.IncludeExecutionHistory != nil && !*options.IncludeExecutionHistory &&
		(options.Limit != nil || (options.PageToken != nil && *options.PageToken != "")) {
		return models.GetOrderResult{}, &sdkerrors.ValidationError{Msg: "orders.Get limit and page_token require include_execution_history"}
	}
	req := &orderv1.GetOrderRequest{
		IncludeAttachedRisk: boolPtr(includeAttachedRisk), IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if options.IncludeExecutionHistory != nil {
		req.IncludeExecutionHistory = options.IncludeExecutionHistory
	}
	if options.Limit != nil {
		req.Limit = options.Limit
	}
	if options.PageToken != nil {
		req.PageToken = *options.PageToken
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.GetOrderResult{}, err
	}
	if err := codecs.ApplyOrderKeyToGet(req, key); err != nil {
		return models.GetOrderResult{}, err
	}
	scales, err := readQuantityScales(ctx, s.catalogs)
	if err != nil {
		return models.GetOrderResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.readClient().GetOrder, req, scaledRead(decode.GetOrderFromProto, decode.ScaleGetOrderResult, scales))
}

// Create places a new order.
func (s *OrdersService) Create(ctx context.Context, req models.CreateOrderRequest, account AccountScope) (models.OrderMutationResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.OrderMutationResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(req.SubAccountID, account)
	if err != nil {
		return models.OrderMutationResult{}, err
	}
	req.SubAccountID = sub
	if err := resolveOrderSymbolID(s.catalogs, &req, nil, "orders.create"); err != nil {
		return models.OrderMutationResult{}, err
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
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CreateOrder, protoReq, func(msg *orderv1.CreateOrderResponse) (models.OrderMutationResult, error) {
		result, err := decode.OrderMutationFromProto(msg)
		if err == nil && result.ResolvedBaseQty != nil {
			qty := result.ResolvedBaseQty.WithScale(scale).WithSymbolID(*req.SymbolID)
			result.ResolvedBaseQty = &qty
		}
		return result, err
	})
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
	sub, err := s.scoped.ResolveSubAccountID(req.SubAccountID, account)
	if err != nil {
		return models.PreviewOrderResult{}, err
	}
	req.SubAccountID = sub
	if err := resolveOrderSymbolID(s.catalogs, &req, nil, "orders.preview"); err != nil {
		return models.PreviewOrderResult{}, err
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
	symbol := ""
	if req.Symbol != nil {
		symbol = *req.Symbol
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().PreviewOrder, protoReq, func(msg *orderv1.PreviewOrderResponse) (models.PreviewOrderResult, error) {
		return decode.PreviewOrderFromProto(msg, baseScale, symbol, req.SymbolID)
	})
}

// Cancel cancels an order. A cancel of an order that has already ended,
// including a repeat of the cancel that ended it, fails with
// ERROR_CODE_ORDER_ALREADY_TERMINAL (errors.IsOrderAlreadyTerminal): treat it
// as already done and read the order for how it ended. A retry by client
// order ID more than a minute after the order ended may read
// ERROR_CODE_ORDER_UNKNOWN; cancel by order ID once you have it. Every cancel,
// a repeat included, counts against the cancel rate limit.
func (s *OrdersService) Cancel(ctx context.Context, account AccountScope, key models.OrderKey, symbol *string, symbolID *uint32, subAccountID *string) (models.OrderMutationResult, error) {
	if symbolID != nil && *symbolID == 0 {
		return models.OrderMutationResult{}, &sdkerrors.ValidationError{Msg: "symbol_id must be non-zero when explicitly supplied"}
	}
	req := &orderv1.CancelOrderRequest{}
	if err := codecs.ApplyOrderKeyToCancel(req, key); err != nil {
		return models.OrderMutationResult{}, err
	}
	if symbol != nil || symbolID != nil {
		id, err := s.resolveSelectors(ctx, symbol, symbolID, "cancel")
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

// Modify modifies an open order's price and/or quantity. Use ModifyWith to
// also replace the order's attached risk.
func (s *OrdersService) Modify(ctx context.Context, account AccountScope, symbol string, key models.OrderKey, subAccountID, requestID *string, newPrice *models.PriceInput, newQty *models.QtyInput, behavior, newClientOrderID *string) (models.ModifyOrderResult, error) {
	return s.ModifyWith(ctx, account, ModifyParams{
		Symbol:           symbol,
		Key:              key,
		SubAccountID:     subAccountID,
		RequestID:        requestID,
		NewPrice:         newPrice,
		NewQty:           newQty,
		Behavior:         behavior,
		NewClientOrderID: newClientOrderID,
	})
}

// ModifyParams is the expanded modify input. Set at least one of NewPrice,
// NewQty, or NewAttachedRisk. NewAttachedRisk replaces the order's attached
// take-profit, stop-loss, or trailing-stop policy. A trailing stop's
// LimitPrice is sent as given: the server knows the order's side, applies the
// side rule and answers ERROR_CODE_BAD_PRICE with no violation.
type ModifyParams struct {
	Symbol           string
	Key              models.OrderKey
	SubAccountID     *string
	RequestID        *string
	NewPrice         *models.PriceInput
	NewQty           *models.QtyInput
	NewAttachedRisk  *models.AttachedRiskInput
	Behavior         *string
	NewClientOrderID *string
}

// ModifyWith modifies an open order from the expanded parameter set.
func (s *OrdersService) ModifyWith(ctx context.Context, account AccountScope, params ModifyParams) (models.ModifyOrderResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.ModifyOrderResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(params.SubAccountID, account)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	symbol := params.Symbol
	resolvedSymbolID, err := ResolveSymbolID(s.catalogs, &symbol, nil, "orders.modify")
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &symbol)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	protoReq, err := codecs.ModifyOrderToProto(
		symbol, resolvedSymbolID, params.Key, sub, params.RequestID,
		params.NewPrice, params.NewQty, params.NewAttachedRisk, params.Behavior, params.NewClientOrderID, scale,
	)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().ModifyOrder, protoReq, decode.ModifyOrderFromProto)
}

// CancelAllParams is the expanded cancel-all filter set.
// Symbol, Symbols, and SymbolIDs are mutually exclusive once any selects a pair.
type CancelAllParams struct {
	SubAccountID *string
	Symbol       *string
	Symbols      []string
	SymbolIDs    []uint32
	Side         *string
	DryRun       bool
	RequestID    *string
}

// CancelAll cancels all matching orders. Empty symbol matches every pair.
// It cancels open orders only, never triggers: the open orders triggers
// placed (a TWAP's working slice, a ladder's levels, a fired stop's order, an
// attached leg's working exit) are canceled like any other, while the
// triggers stay live and keep their reserved funds. MatchedOrders counts
// orders, never triggers. To stop all automated trading, use
// StopAllTrading, which cancels the triggers first.
func (s *OrdersService) CancelAll(ctx context.Context, account AccountScope, subAccountID, symbol, side *string, dryRun bool, requestID *string) (models.CancelAllOrdersResult, error) {
	return s.CancelAllWith(ctx, account, CancelAllParams{
		SubAccountID: subAccountID,
		Symbol:       symbol,
		Side:         side,
		DryRun:       dryRun,
		RequestID:    requestID,
	})
}

// CancelAllWith cancels matching orders for one or more pairs.
func (s *OrdersService) CancelAllWith(ctx context.Context, account AccountScope, params CancelAllParams) (models.CancelAllOrdersResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(params.SubAccountID, account)
	if err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	if namesSymbol(params.Symbol, params.Symbols...) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.CancelAllOrdersResult{}, err
		}
	}
	symbolIDs, err := ResolveCancelAllSymbolIDs(s.catalogs, params.Symbol, params.Symbols, params.SymbolIDs, "orders.cancel_all")
	if err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	protoReq, err := codecs.CancelAllOrdersToProto(sub, symbolIDs, params.Side, params.DryRun, params.RequestID)
	if err != nil {
		return models.CancelAllOrdersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().CancelAllOrders, protoReq, decode.CancelAllFromProto)
}

// CancelAllAfter arms cancel-all-after (the dead-man switch). When it fires
// it cancels open orders in scope, never triggers: a client that stopped
// still has its triggers live. Cancel them with Triggers.Cancel, or use
// StopAllTrading.
func (s *OrdersService) CancelAllAfter(ctx context.Context, account AccountScope, timeoutSec int, subAccountID, symbol, side, requestID *string) (models.CancelAllAfterResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.CancelAllAfterResult{}, err
	}
	if namesSymbol(symbol) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.CancelAllAfterResult{}, err
		}
	}
	resolvedSymbolID, err := ResolveOptionalSymbolID(s.catalogs, symbol, nil, "orders.cancel_all_after")
	if err != nil {
		return models.CancelAllAfterResult{}, err
	}
	var symbolID *uint32
	if resolvedSymbolID != 0 {
		symbolID = uint32Ptr(resolvedSymbolID)
	}
	protoReq, err := codecs.CancelAllAfterToProto(sub, timeoutSec, symbolID, side, requestID)
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
	// Resolve every scale from the ID that is actually sent for that item.
	resolvedItems := append([]models.CreateOrderRequest(nil), items...)
	itemScales := make([]codecs.OrderScales, len(items))
	for i := range resolvedItems {
		item := &resolvedItems[i]
		if err := resolveOrderSymbolID(s.catalogs, item, symbol, "orders.batch_create"); err != nil {
			return models.BatchCreateOrdersResult{}, err
		}
		base, err := quantityScaleForOrderWrite(s.catalogs, item.Symbol, item.SymbolID)
		if err != nil {
			return models.BatchCreateOrdersResult{}, err
		}
		quote, err := quoteQuantityScaleForOrderWrite(s.catalogs, item.Symbol, item.SymbolID, item.MaxQuoteDebitScaled.IsSet())
		if err != nil {
			return models.BatchCreateOrdersResult{}, err
		}
		itemScales[i] = codecs.OrderScales{Base: base, Quote: quote}
	}
	protoReq, err := codecs.BatchCreateOrdersToProto(resolvedItems, sub, requestID, allowPartial, 0, 0, itemScales...)
	if err != nil {
		return models.BatchCreateOrdersResult{}, err
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
// when retrying the same logical batch: an identical retry replays the
// admission for 60 seconds from acceptance. Read the status within that
// window; after it, reconcile the individual orders and never send an
// accepted batch again.
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
	protoReq, err := codecs.BatchReplaceOrdersToProto(items, symbolID, sub, requestID, scale, symbol)
	if err != nil {
		return models.BatchReplaceOrdersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().BatchReplaceOrders, protoReq, decode.BatchReplaceFromProto)
}

// GetBatchReplaceStatus returns the execution status of one accepted batch
// replacement: its admission and each item's current phase. A working
// successor is live, but not necessarily execution-final.
//
// The status is retained for 60 seconds from the batch's acceptance and is
// readable as soon as BatchReplace returns; each item's phase then advances
// asynchronously. batchRequestID is the exact 64-bit BatchRequestID that
// BatchReplace returned.
//
//   - UNAVAILABLE (*errors.ServerError, retryable) means the status can't be
//     established yet: send the same request again within the window.
//   - Not found (*errors.APIError with Code "not_found") means the account has
//     no retained batch with this ID: none was accepted for it, or the window
//     ended. It never proves that a replacement was rejected.
//
// After the window, reconcile the individual orders with Orders.Get, by each
// item's old and replacement order IDs, and never send an accepted batch again
// because its status is not found: a batch sent after the window is admitted
// as a new one.
//
// An item's UpdatedTsNs is an observation time: for an item refused after its
// admission it can be a later time at which the service recovered and
// observed the refusal, so two reads may differ for the same refusal. Don't
// use it to identify the refusal.
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

// batchReplaceStatusWindow is how long GetBatchReplaceStatus keeps a batch's
// status, from its acceptance.
var batchReplaceStatusWindow = 60 * time.Second

// batchReplacePoll is the wait between WaitBatchReplace status reads.
var batchReplacePoll = unavailableRetry{first: 50 * time.Millisecond, max: 500 * time.Millisecond}

// WaitBatchReplace polls GetBatchReplaceStatus until no item is still
// admitted, that is until each is working, rejected or terminal, and returns
// that status. Waits start at 50 ms and double up to 500 ms. A read answered
// UNAVAILABLE or another 5xx is polled again. A read answered not found is
// polled again too, because an older API could answer not
// found for a few seconds after admission; not found never means the batch
// was rejected.
//
// A rejected item changed nothing, including one admitted and then refused
// when applied: its order keeps OldOrderID, and Code and ErrorCode name the
// refusal. Only a status item's ReplacementOrderID, or an order read, proves
// a successor order; the planned ID in BatchReplace's admission answer does
// not.
//
// The status is kept for 60 s from the batch's acceptance. When that window
// ends with an item still admitted, WaitBatchReplace returns the last status
// with *errors.BatchReplaceWindowExpiredError: reconcile the orders with order
// reads. A read still in flight when the window ends is abandoned. Bound the
// wait further with ctx; its cancellation or deadline returns ctx.Err().
func (s *OrdersService) WaitBatchReplace(ctx context.Context, account AccountScope, batchRequestID string, subAccountID *string) (models.BatchReplaceStatusResult, error) {
	deadline := time.Now().Add(batchReplaceStatusWindow)
	delay := batchReplacePoll.first
	var last models.BatchReplaceStatusResult
	for {
		// Bound the read itself by the window: a slow answer must not keep
		// the wait past it.
		readCtx, cancelRead := context.WithDeadline(ctx, deadline)
		status, err := s.GetBatchReplaceStatus(readCtx, account, batchRequestID, subAccountID)
		readExpired := readCtx.Err() != nil
		cancelRead()
		switch {
		case ctx.Err() != nil:
			// The caller canceled the wait or its own deadline passed.
			return last, ctx.Err()
		case err != nil && readExpired:
			return last, &sdkerrors.BatchReplaceWindowExpiredError{BatchRequestID: batchRequestID}
		case err == nil:
			last = status
			if status.AcceptedTsNs != 0 {
				deadline = time.Unix(0, int64(status.AcceptedTsNs)).Add(batchReplaceStatusWindow)
			}
			if models.IsBatchReplaceSettled(status) {
				return status, nil
			}
		case readUnavailable(err) || batchStatusNotFound(err):
			// Not readable yet: a new batch's status can read not found
			// for a few seconds after admission. Read again within the
			// window.
		default:
			return last, err
		}
		if time.Now().Add(delay).After(deadline) {
			return last, &sdkerrors.BatchReplaceWindowExpiredError{BatchRequestID: batchRequestID}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
		delay = min(2*delay, batchReplacePoll.max)
	}
}

// batchStatusNotFound reports a status read answered not found.
func batchStatusNotFound(err error) bool {
	var apiErr *sdkerrors.APIError
	return errors.As(err, &apiErr) &&
		(apiErr.Code == connect.CodeNotFound.String() || apiErr.Code == orderv1.ErrorCode_ERROR_CODE_NOT_FOUND.String())
}

// readUnavailable reports a transient server or transport failure of a read.
// A failed catalog load is never transient: the client never loads its
// catalogs again, whatever the load failed with.
func readUnavailable(err error) bool {
	var hydrationErr *sdkerrors.CatalogHydrationError
	if errors.As(err, &hydrationErr) {
		return false
	}
	var serverErr *sdkerrors.ServerError
	var transportErr *sdkerrors.TransportError
	var apiErr *sdkerrors.APIError
	return errors.As(err, &serverErr) || errors.As(err, &transportErr) ||
		(errors.As(err, &apiErr) && apiErr.Code == orderv1.ErrorCode_ERROR_CODE_UNAVAILABLE.String())
}

// Subscribe streams an account's orders over the private WebSocket until the
// subscription is closed or ctx is canceled. accountID is a public ID string
// or a positive integer, which is encoded as its public ID (42 is "j"), and
// defaults to the client's account ID when nil or "".
//
// Events arrive on Messages(): OrderStreamConnected, then an OrderSnapshot of
// the live view, then OrderUnit changes, plus OrderStale, OrderStreamDisconnected
// and OrderStreamError. Orders decode to the same models.Order as the order
// reads; keep the copy with the higher Version. Gaps, expiring tokens and
// reconnects resume from the last applied position.
func (s *OrdersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[realtime.OrderStreamEvent], error) {
	if err := requireRealtime(s.realtime); err != nil {
		return nil, err
	}
	resolved, err := ResolveAccountID(accountID, s.defaultAccountID)
	if err != nil {
		return nil, err
	}
	return s.realtime.SubscribeOrders(ctx, resolved)
}

// WaitForOrderTradesComplete polls GetOrder until the order is terminal
// (Status "filled", "canceled" or "rejected") and the sum of returned trade
// quantities equals its cumulative filled quantity, or the timeout elapses. A
// timeout of zero or less waits 15 s.
//
// Trade projection can lag the order filled/cum qty fields; use this helper
// when an order's fills must be final and fully projected before continuing.
// ExecutionsComplete alone does not mean that: it is also true for a working
// order whose fills so far are all recorded, for example after a partial fill,
// so such an order keeps the wait polling until it ends or the timeout passes.
// To check the trades recorded so far on a working order, read it with Get
// and compare its trades to CumQty. Each poll reads every execution page, and
// the result carries the order, trades and transfers of the last poll with
// that read's ExecutionsComplete.
//
// The timeout bounds the whole wait, page reads included: a read still in
// flight when it passes is abandoned, a read that finishes after it is no
// success, and the wait returns the last poll's result with an error that
// wraps context.DeadlineExceeded. A
// next_page_token that repeats a page token of the same poll returns
// *errors.ResponseContractError. A ctx cancellation or deadline returns
// ctx.Err().
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
	waitCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	// expired reports the wait's own deadline as passed. A page read it cuts
	// short can fail before waitCtx says so: Connect sends the server the
	// remaining time in whole milliseconds, so the server's copy of the
	// deadline can pass up to 1 ms earlier.
	expired := func() bool {
		return waitCtx.Err() != nil || time.Until(deadline) < time.Millisecond
	}
	var last models.GetOrderResult
	timedOut := func() error {
		cum := int64(0)
		if last.Order != nil {
			cum = last.Order.CumQty.Scaled()
		}
		return fmt.Errorf(
			"order trades not complete within %s: cum_qty=%d trade_qty_sum=%d trades=%d: %w",
			timeout, cum, sumTradeQty(last.Trades), len(last.Trades), context.DeadlineExceeded,
		)
	}
	for {
		detail, err := s.getOrderExecutionPages(waitCtx, account, key, subAccountID)
		switch {
		case ctx.Err() != nil:
			return last, ctx.Err()
		case err != nil && expired():
			// The timeout cut a page read short.
			return last, timedOut()
		case err != nil:
			return last, err
		}
		last = detail
		// A read answered in time can still finish after the deadline: it
		// is then the last result of a wait that timed out, not a success.
		if !time.Now().Before(deadline) {
			return last, timedOut()
		}
		if detail.Order != nil && orderTradesComplete(detail) {
			return detail, nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return last, ctx.Err()
			}
			return last, timedOut()
		case <-timer.C:
		}
	}
}

// getOrderExecutionPages reads one order with every page of its execution
// history and returns the order, the trades and transfers of all pages, and
// the final page's ExecutionsComplete.
func (s *OrdersService) getOrderExecutionPages(
	ctx context.Context,
	account AccountScope,
	key models.OrderKey,
	subAccountID *string,
) (models.GetOrderResult, error) {
	includeHistory := true
	var pageToken *string
	var trades []models.UserTrade
	var transfers []models.OrderTransfer
	var order *models.Order
	requested := map[string]bool{}
	for {
		page, err := s.Get(ctx, account, key, subAccountID, false, false, models.GetOrderOptions{
			IncludeExecutionHistory: &includeHistory,
			PageToken:               pageToken,
		})
		if err != nil {
			return models.GetOrderResult{}, err
		}
		if page.Order != nil {
			if page.Order.OrderID != "" {
				key = models.OrderKeyByID(page.Order.OrderID)
			}
			order = page.Order
		}
		trades = append(trades, page.Trades...)
		transfers = append(transfers, page.Transfers...)
		if page.NextPageToken == "" {
			return models.GetOrderResult{
				Order:              order,
				Trades:             trades,
				Transfers:          transfers,
				ExecutionsComplete: page.ExecutionsComplete,
			}, nil
		}
		if requested[page.NextPageToken] {
			return models.GetOrderResult{}, &sdkerrors.ResponseContractError{
				Operation: "GetOrder",
				Msg:       "next_page_token repeats an earlier page token",
			}
		}
		token := page.NextPageToken
		requested[token] = true
		pageToken = &token
	}
}

// orderTradesComplete reports an order that has ended and whose trades add up
// to its cumulative filled quantity: no further fill can follow.
func orderTradesComplete(detail models.GetOrderResult) bool {
	if detail.Order == nil {
		return false
	}
	switch detail.Order.Status {
	case "filled", "canceled", "rejected":
	default:
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

func resolveOrderSymbolID(c *catalogs.Manager, req *models.CreateOrderRequest, fallback *string, label string) error {
	symbol := req.Symbol
	if symbol == nil && req.SymbolID == nil {
		symbol = fallback
	}
	id, err := ResolveSymbolID(c, symbol, req.SymbolID, label)
	if err != nil {
		return err
	}
	if id == 0 {
		return &sdkerrors.ValidationError{Msg: label + " requires a resolved non-zero symbol_id"}
	}
	if c == nil {
		return &sdkerrors.ValidationError{Msg: label + " quantity scale is unavailable; hydrate catalogs"}
	}
	canonical := c.SymbolForSymbolID(id)
	if canonical == nil {
		return &sdkerrors.ValidationError{Msg: label + " quantity scale is unavailable for symbol_id; hydrate catalogs"}
	}
	if req.Symbol != nil && *req.Symbol != *canonical {
		return &sdkerrors.ValidationError{Msg: label + " symbol mismatch: supplied symbol does not match symbol_id"}
	}
	req.SymbolID = &id
	req.Symbol = canonical
	return nil
}

func quantityScaleForOrderWrite(c *catalogs.Manager, symbol *string, symbolID *uint32) (int, error) {
	if symbolID == nil {
		return codecs.QuantityScaleForSymbol(c, symbol)
	}
	if c != nil {
		scale, ok := c.BaseQuantityScaleForSymbolID(*symbolID)
		if !ok {
			return 0, &sdkerrors.ValidationError{
				Msg: "quantity scale for symbol_id is unavailable; call WaitForCatalogs before placing orders",
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
	if symbolID == nil {
		return codecs.QuoteQuantityScaleForSymbol(c, symbol)
	}
	if c != nil {
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

// scaleOrdersList attaches the pair's base quantity scale to each listed order.
func scaleOrdersList(list models.OrdersList, scales decode.BaseQuantityScales) models.OrdersList {
	decode.ScaleOrders(list.Orders, scales)
	return list
}
