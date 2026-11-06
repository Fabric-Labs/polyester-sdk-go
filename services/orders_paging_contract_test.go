package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type executionPageHandler struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	calls []*orderv1.GetOrderRequest
}

func (h *executionPageHandler) GetOrder(_ context.Context, r *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	h.calls = append(h.calls, r.Msg)
	next := ""
	qty := int64(60)
	if len(h.calls) == 1 {
		next = "older-executions"
		qty = 40
	}
	return connect.NewResponse(&orderv1.GetOrderResponse{Order: &orderv1.Order{OrderId: 9, CumQtyScaled: 100, InheritedCumQtyScaled: 40}, Trades: []*orderv1.UserTrade{{OrderId: 9, QtyScaled: qty}}, NextPageToken: next}), nil
}
func TestExecutionPagingPinsOrderGeneration(t *testing.T) {
	handler := &executionPageHandler{}
	_, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	srv := hardening.NewTestServer(h)
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	service := NewOrdersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil)
	result, err := service.getOrderExecutionPages(context.Background(), nil, models.OrderKeyByClientID("reused-client-id"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(handler.calls) != 2 || handler.calls[0].GetClientOrderId() != "reused-client-id" || handler.calls[1].GetOrderId() != 9 || handler.calls[1].GetPageToken() != "older-executions" {
		t.Fatalf("cursor was not pinned to returned order: %v", handler.calls)
	}
	if result.Order.OrderID != codecs.FormatID(9) || !orderTradesComplete(result) {
		t.Fatalf("lineage history did not reconcile: %+v", result)
	}
}
