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
func (s *OrdersService) ListOpen(ctx context.Context, account AccountScope, subAccountID *string, pageToken *string, limit *int, includeAttachedRisk, includeAttachedRiskState bool) (models.OrdersList, error) {
	req := &orderv1.GetOpenOrdersRequest{
		IncludeAttachedRisk: boolPtr(includeAttachedRisk), IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.OrdersList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	if limit != nil {
		req.Limit = uint32Ptr(uint32(*limit))
	}
	return UnaryAuth(ctx, s.transport, s.readClient().GetOpenOrders, req, decode.OrdersListFromOpen)
}

// ListHistory returns order history.
func (s *OrdersService) ListHistory(ctx context.Context, account AccountScope, subAccountID, symbol *string, symbolID *uint32, pageToken *string, limit int, includeAttachedRisk, includeAttachedRiskState bool) (models.OrdersList, error) {
	req := &orderv1.GetOrderHistoryRequest{
		Limit: uint32Ptr(uint32(limit)), IncludeAttachedRisk: boolPtr(includeAttachedRisk),
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
	return UnaryAuth(ctx, s.transport, s.readClient().GetOrderHistory, req, decode.OrdersListFromHistory)
}

// Get returns one order.
func (s *OrdersService) Get(ctx context.Context, account AccountScope, orderID, clientOrderID, subAccountID *string, includeAttachedRisk, includeAttachedRiskState bool) (models.GetOrderResult, error) {
	if (orderID == nil || *orderID == "") && (clientOrderID == nil || *clientOrderID == "") {
		return models.GetOrderResult{}, &sdkerrors.ValidationError{Msg: "get requires order_id or client_order_id"}
	}
	req := &orderv1.GetOrderRequest{
		IncludeAttachedRisk: boolPtr(includeAttachedRisk), IncludeAttachedRiskState: boolPtr(includeAttachedRiskState),
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.GetOrderResult{}, err
	}
	if orderID != nil {
		id, err := codecs.IDToInt(*orderID, "order_id")
		if err != nil {
			return models.GetOrderResult{}, err
		}
		req.Key = &orderv1.GetOrderRequest_OrderId{OrderId: id}
	}
	if clientOrderID != nil {
		req.Key = &orderv1.GetOrderRequest_ClientOrderId{ClientOrderId: *clientOrderID}
	}
	return UnaryAuth(ctx, s.transport, s.readClient().GetOrder, req, decode.GetOrderFromProto)
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
	protoReq, err := codecs.CreateOrderToProto(req, scale)
	if err != nil {
		return models.OrderMutationResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().CreateOrder, protoReq, decode.OrderMutationFromProto)
}

// Cancel cancels an order.
func (s *OrdersService) Cancel(ctx context.Context, account AccountScope, orderID, clientOrderID, symbol *string, symbolID *uint32, subAccountID *string) (models.OrderMutationResult, error) {
	if (orderID == nil || *orderID == "") && (clientOrderID == nil || *clientOrderID == "") {
		return models.OrderMutationResult{}, &sdkerrors.ValidationError{Msg: "cancel requires order_id or client_order_id"}
	}
	req := &orderv1.CancelOrderRequest{}
	if orderID != nil {
		id, err := codecs.IDToInt(*orderID, "order_id")
		if err != nil {
			return models.OrderMutationResult{}, err
		}
		req.Key = &orderv1.CancelOrderRequest_OrderId{OrderId: id}
	}
	if clientOrderID != nil {
		req.Key = &orderv1.CancelOrderRequest_ClientOrderId{ClientOrderId: *clientOrderID}
	}
	if symbolID != nil {
		req.SymbolId = *symbolID
	} else if symbol != nil {
		if id := s.catalogs.SymbolIDForSymbol(*symbol); id != nil {
			req.SymbolId = *id
		}
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.OrderMutationResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().CancelOrder, req, decode.OrderMutationFromCancel)
}

// Modify modifies an open order.
func (s *OrdersService) Modify(ctx context.Context, account AccountScope, symbol string, orderID, clientOrderID, subAccountID, requestID *string, newPrice *models.PriceInput, newQty *models.QtyInput, behavior, newClientOrderID *string) (models.ModifyOrderResult, error) {
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
	protoReq, err := codecs.ModifyOrderToProto(symbol, orderID, clientOrderID, sub, requestID, newPrice, newQty, behavior, newClientOrderID, scale)
	if err != nil {
		return models.ModifyOrderResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().ModifyOrder, protoReq, decode.ModifyOrderFromProto)
}

// CancelAll cancels all matching orders.
func (s *OrdersService) CancelAll(ctx context.Context, account AccountScope, subAccountID, symbol, side *string, dryRun bool, requestID *string) (models.CancelAllOrdersResult, error) {
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
	protoReq, err := codecs.BatchCreateOrdersToProto(items, sub, requestID, allowPartial, scale)
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

// BatchModify modifies multiple orders in one request.
func (s *OrdersService) BatchModify(ctx context.Context, account AccountScope, items []models.BatchModifyItem, subAccountID *string, symbol *string, requestID *string, behaviorDefault *string, allowPartial bool) (models.BatchModifyOrdersResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.BatchModifyOrdersResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BatchModifyOrdersResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, symbol)
	if err != nil {
		return models.BatchModifyOrdersResult{}, err
	}
	protoReq, err := codecs.BatchModifyOrdersToProto(items, sub, requestID, behaviorDefault, allowPartial, scale)
	if err != nil {
		return models.BatchModifyOrdersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().BatchModifyOrders, protoReq, decode.BatchModifyFromProto)
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
	orderID, clientOrderID, subAccountID *string,
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
		detail, err := s.Get(ctx, account, orderID, clientOrderID, subAccountID, false, false)
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
				cum = detail.Order.CumQty.Scaled
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
	return sumTradeQty(detail.Trades) == detail.Order.CumQty.Scaled
}

func sumTradeQty(trades []models.UserTrade) int64 {
	var sum int64
	for _, trade := range trades {
		sum += trade.Qty.Scaled
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
