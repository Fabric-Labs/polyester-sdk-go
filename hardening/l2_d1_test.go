package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
)

type getOrderSeq struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	calls atomic.Int64
}

func (h *getOrderSeq) GetOrder(context.Context, *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	n := h.calls.Add(1)
	order := &orderv1.Order{
		OrderId:      1,
		SymbolId:     1,
		CumQtyScaled: 100,
	}
	if n == 1 {
		return connect.NewResponse(&orderv1.GetOrderResponse{
			Order:  order,
			Trades: nil,
		}), nil
	}
	return connect.NewResponse(&orderv1.GetOrderResponse{
		Order: order,
		Trades: []*orderv1.UserTrade{
			{SymbolId: 1, QtyScaled: 40},
			{SymbolId: 1, QtyScaled: 60},
		},
	}), nil
}

func TestWaitForOrderTradesCompletePollsUntilSumMatches(t *testing.T) {
	handler := &getOrderSeq{}
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	mux.Handle(path, h)
	var httpHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHits.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: false,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	orderID := "1"
	result, err := client.Orders.WaitForOrderTradesComplete(
		context.Background(),
		nil,
		&orderID,
		nil,
		nil,
		2*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if handler.calls.Load() < 2 {
		t.Fatalf("expected at least 2 GetOrder polls, got %d", handler.calls.Load())
	}
	if httpHits.Load() < 2 {
		t.Fatalf("WaitForOrderTradesComplete must cross Connect/HTTP; http hits=%d", httpHits.Load())
	}
	var sum int64
	for _, tr := range result.Trades {
		sum += tr.Qty.Scaled
	}
	if result.Order == nil || result.Order.CumQty.Scaled != 100 || sum != 100 {
		t.Fatalf("cum=%v sum=%d trades=%+v", result.Order, sum, result.Trades)
	}
}
