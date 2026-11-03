package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// stopAllServer records every trigger and order call in the order received.
type stopAllServer struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	ordersv1connect.UnimplementedOrdersServiceHandler
	mu        sync.Mutex
	calls     []string
	lists     []*triggersv1.ListTriggersRequest
	cancelAll []*orderv1.CancelAllOrdersRequest
	refuse    uint64
}

func (s *stopAllServer) ListTriggers(_ context.Context, req *connect.Request[triggersv1.ListTriggersRequest]) (*connect.Response[triggersv1.ListTriggersResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "list:"+req.Msg.GetPageToken())
	s.lists = append(s.lists, req.Msg)
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&triggersv1.ListTriggersResponse{
			Triggers:      []*triggersv1.Trigger{{TriggerId: 1}, {TriggerId: 2}},
			NextPageToken: "p2",
		}), nil
	}
	return connect.NewResponse(&triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{TriggerId: 3}}}), nil
}

func (s *stopAllServer) CancelTrigger(_ context.Context, req *connect.Request[triggersv1.CancelTriggerRequest]) (*connect.Response[triggersv1.CancelTriggerResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "cancel-trigger")
	if req.Msg.GetTriggerId() == s.refuse {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("trigger already ended"))
	}
	return connect.NewResponse(&triggersv1.CancelTriggerResponse{
		TriggerId: req.Msg.GetTriggerId(), Status: triggersv1.TriggerStatus_STATUS_CANCELED,
	}), nil
}

func (s *stopAllServer) CancelAllOrders(_ context.Context, req *connect.Request[orderv1.CancelAllOrdersRequest]) (*connect.Response[orderv1.CancelAllOrdersResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "cancel-all")
	s.cancelAll = append(s.cancelAll, req.Msg)
	return connect.NewResponse(&orderv1.CancelAllOrdersResponse{
		Status: orderv1.CancelAllOrdersResponse_SUBMITTED, MatchedOrders: 4, SubmittedCancels: 4,
	}), nil
}

func newStopAllServices(t *testing.T, server *stopAllServer) (*TriggersService, *OrdersService) {
	t.Helper()
	mux := http.NewServeMux()
	triggersPath, triggersHandler := triggersv1connect.NewTriggersServiceHandler(server)
	mux.Handle(triggersPath, triggersHandler)
	ordersPath, ordersHandler := ordersv1connect.NewOrdersServiceHandler(server)
	mux.Handle(ordersPath, ordersHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	factory := transport.NewFactory(cfg, creds, nil)
	return NewTriggersService(factory, cats, nil, nil, nil, nil, nil), NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
}

// Every live trigger is canceled, across pages, before the open orders: an
// order a trigger places before its cancel is still canceled.
func TestStopAllTradingCancelsTriggersThenOrders(t *testing.T) {
	server := &stopAllServer{}
	triggers, orders := newStopAllServices(t, server)
	symbol := "BTC-USDT"
	requestID := "stop-all-1"

	result, err := StopAllTrading(context.Background(), triggers, orders, nil, StopAllTradingScope{Symbol: &symbol, RequestID: &requestID})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"list:", "list:p2", "cancel-trigger", "cancel-trigger", "cancel-trigger", "cancel-all"}
	if !reflect.DeepEqual(server.calls, want) {
		t.Fatalf("calls = %v, want %v", server.calls, want)
	}
	live := []triggersv1.TriggerStatus{
		triggersv1.TriggerStatus_STATUS_CREATED, triggersv1.TriggerStatus_STATUS_ARMED,
		triggersv1.TriggerStatus_STATUS_RUNNING, triggersv1.TriggerStatus_STATUS_PAUSED,
	}
	for _, list := range server.lists {
		if !reflect.DeepEqual(list.GetStatus(), live) || list.GetLimit() != 1000 || list.GetSymbolId() != 5 {
			t.Fatalf("list request = %v", list)
		}
	}
	if got := server.cancelAll[0]; !reflect.DeepEqual(got.GetSymbolIds(), []uint32{5}) || got.GetRequestId() != requestID {
		t.Fatalf("cancel-all request = %v", got)
	}
	if len(result.CanceledTriggers) != 3 || len(result.FailedTriggers) != 0 || result.CancelAll.MatchedOrders != 4 {
		t.Fatalf("result = %+v", result)
	}
}

// A trigger cancel that fails does not stop the others, nor the order
// cancel-all; the failure is reported.
func TestStopAllTradingReportsTriggerCancelFailures(t *testing.T) {
	server := &stopAllServer{refuse: 2}
	triggers, orders := newStopAllServices(t, server)

	result, err := StopAllTrading(context.Background(), triggers, orders, nil, StopAllTradingScope{})
	if err == nil {
		t.Fatal("expected the failed trigger cancel to be reported")
	}
	if server.calls[len(server.calls)-1] != "cancel-all" || len(result.CanceledTriggers) != 2 ||
		len(result.FailedTriggers) != 1 || result.FailedTriggers[0].TriggerID == "" || result.FailedTriggers[0].Err == nil {
		t.Fatalf("calls = %v result = %+v", server.calls, result)
	}
}
