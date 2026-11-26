package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
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
	return connect.NewResponse(&orderv1.GetOrderResponse{Order: &orderv1.Order{OrderId: 9, ClientOrderId: "reused-client-id", Status: orderv1.OrderStatus_FILLED, CumQtyScaled: 100, InheritedCumQtyScaled: 40}, Trades: []*orderv1.UserTrade{{OrderId: 9, QtyScaled: qty}}, NextPageToken: next}), nil
}
func TestExecutionPagingPinsOrderGeneration(t *testing.T) {
	handler := &executionPageHandler{}
	_, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	srv := hardening.NewTestServer(h)
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
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

// sharedMatchPages answers GetOrder with two execution pages whose fills
// share match 77, so the match's two settlement legs appear on both pages, as
// the contract allows. The second page also settles match 78.
type sharedMatchPages struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
}

func (sharedMatchPages) GetOrder(_ context.Context, r *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	order := &orderv1.Order{OrderId: 9, Status: orderv1.OrderStatus_FILLED, CumQtyScaled: 100}
	legs := []*orderv1.OrderTransfer{
		{MatchId: 77, SymbolId: 1, AssetId: 1, TxId: "tx-77-base"},
		{MatchId: 77, SymbolId: 1, AssetId: 2, IsDebit: true, TxId: "tx-77-quote"},
	}
	if r.Msg.GetPageToken() == "" {
		return connect.NewResponse(&orderv1.GetOrderResponse{
			Order:         order,
			Trades:        []*orderv1.UserTrade{{OrderId: 9, MatchId: 77, SymbolId: 1, QtyScaled: 60}},
			Transfers:     legs,
			NextPageToken: "p2",
		}), nil
	}
	return connect.NewResponse(&orderv1.GetOrderResponse{
		Order:  order,
		Trades: []*orderv1.UserTrade{{OrderId: 9, MatchId: 77, SymbolId: 1, QtyScaled: 30}, {OrderId: 9, MatchId: 78, SymbolId: 1, QtyScaled: 10}},
		Transfers: append(legs,
			&orderv1.OrderTransfer{MatchId: 78, SymbolId: 1, AssetId: 1, TxId: "tx-78-base"},
			&orderv1.OrderTransfer{MatchId: 78, SymbolId: 1, AssetId: 2, IsDebit: true, TxId: "tx-78-quote"},
		),
		ExecutionsComplete: true,
	}), nil
}

// A match whose legs appear on two execution pages contributes each leg once,
// at its first position; legs of other matches on later pages are kept.
func TestWaitForOrderTradesCompleteKeepsEachSettlementLegOnce(t *testing.T) {
	_, h := ordersv1connect.NewOrdersReadServiceHandler(sharedMatchPages{})
	srv := hardening.NewTestServer(h)
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	service := NewOrdersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil)
	result, err := service.WaitForOrderTradesComplete(context.Background(), nil, models.OrderKeyByID("A"), nil, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 3 {
		t.Fatalf("trades = %d, want 3 distinct fills", len(result.Trades))
	}
	var got []string
	for _, leg := range result.Transfers {
		got = append(got, leg.TxID)
	}
	want := []string{"tx-77-base", "tx-77-quote", "tx-78-base", "tx-78-quote"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("transfers = %v, want %v", got, want)
	}
}
