package services

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
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
	// tokens, when set, are the next page tokens of successive ListTriggers
	// answers; an exhausted list ends the pagination.
	tokens []string
}

// stopAllListCap stops a fixture that keeps paging, so a missed cycle fails
// the test instead of hanging it.
const stopAllListCap = 12

func (s *stopAllServer) ListTriggers(_ context.Context, req *connect.Request[triggersv1.ListTriggersRequest]) (*connect.Response[triggersv1.ListTriggersResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "list:"+req.Msg.GetPageToken())
	s.lists = append(s.lists, req.Msg)
	if s.tokens != nil {
		if len(s.lists) > stopAllListCap {
			return nil, connect.NewError(connect.CodeAborted, errors.New("fixture stopped a pagination loop"))
		}
		next := ""
		if i := len(s.lists) - 1; i < len(s.tokens) {
			next = s.tokens[i]
		}
		return connect.NewResponse(&triggersv1.ListTriggersResponse{
			Triggers:      []*triggersv1.Trigger{{TriggerId: uint64(len(s.lists))}},
			NextPageToken: next,
		}), nil
	}
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
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
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

// A next page token seen earlier in the same run, not only the one just
// sent, is a pagination cycle: StopAllTrading stops listing, cancels
// nothing, and says so. A finite listing still cancels everything once.
func TestStopAllTradingRefusesPaginationCycles(t *testing.T) {
	// Each cycle repeats past the fixture cap, as a server stuck in a loop
	// would; the names list the tokens StopAllTrading should read.
	cycle := func(tokens ...string) []string {
		out := append([]string(nil), tokens...)
		repeated := tokens[len(tokens)-1]
		first := 0
		for tokens[first] != repeated {
			first++
		}
		for len(out) < 2*stopAllListCap {
			out = append(out, tokens[first+1:]...)
		}
		return out
	}
	cycles := map[string][]string{
		"A,A":       cycle("A", "A"),
		"A,B,A":     cycle("A", "B", "A"),
		"A,B,C,A":   cycle("A", "B", "C", "A"),
		"A,B,C,D,B": cycle("A", "B", "C", "D", "B"),
	}
	for name, tokens := range cycles {
		t.Run(name, func(t *testing.T) {
			server := &stopAllServer{tokens: tokens}
			triggers, orders := newStopAllServices(t, server)

			result, err := StopAllTrading(context.Background(), triggers, orders, nil, StopAllTradingScope{})
			var contractErr *sdkerrors.ResponseContractError
			if !errors.As(err, &contractErr) {
				t.Fatalf("err = %T %v, want ResponseContractError; calls = %v", err, err, server.calls)
			}
			if !strings.Contains(err.Error(), "repeats an earlier page token") || !strings.Contains(err.Error(), "nothing was canceled") {
				t.Fatalf("err = %v", err)
			}
			for _, call := range server.calls {
				if !strings.HasPrefix(call, "list:") {
					t.Fatalf("a cancel began after a pagination cycle: %v", server.calls)
				}
			}
			if wantLists := len(strings.Split(name, ",")); len(server.lists) != wantLists {
				t.Fatalf("lists = %d, want %d: %v", len(server.lists), wantLists, server.calls)
			}
			if len(result.CanceledTriggers) != 0 || len(result.FailedTriggers) != 0 || result.CancelAll.MatchedOrders != 0 {
				t.Fatalf("result = %+v", result)
			}
		})
	}

	server := &stopAllServer{tokens: []string{"A", "B"}}
	triggers, orders := newStopAllServices(t, server)
	result, err := StopAllTrading(context.Background(), triggers, orders, nil, StopAllTradingScope{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"list:", "list:A", "list:B", "cancel-trigger", "cancel-trigger", "cancel-trigger", "cancel-all"}
	if !reflect.DeepEqual(server.calls, want) || len(result.CanceledTriggers) != 3 {
		t.Fatalf("calls = %v, want %v; result = %+v", server.calls, want, result)
	}
}
