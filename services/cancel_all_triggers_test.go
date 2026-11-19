package services

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

type cancelAllHandler struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
	cancelAll      []*orderv1.CancelAllOrdersRequest
	cancelAllAfter []*orderv1.CancelAllAfterRequest
}

func (h *cancelAllHandler) CancelAllOrders(_ context.Context, req *connect.Request[orderv1.CancelAllOrdersRequest]) (*connect.Response[orderv1.CancelAllOrdersResponse], error) {
	h.cancelAll = append(h.cancelAll, proto.Clone(req.Msg).(*orderv1.CancelAllOrdersRequest))
	resp := &orderv1.CancelAllOrdersResponse{Status: orderv1.CancelAllOrdersResponse_SUBMITTED}
	if req.Msg.GetIncludeTriggers() {
		resp.MatchedTriggers = 3
	}
	return connect.NewResponse(resp), nil
}

func (h *cancelAllHandler) CancelAllAfter(_ context.Context, req *connect.Request[orderv1.CancelAllAfterRequest]) (*connect.Response[orderv1.CancelAllAfterResponse], error) {
	h.cancelAllAfter = append(h.cancelAllAfter, proto.Clone(req.Msg).(*orderv1.CancelAllAfterRequest))
	return connect.NewResponse(&orderv1.CancelAllAfterResponse{Status: orderv1.CancelAllAfterResponse_ARMED, EffectiveTimeoutSec: req.Msg.GetTimeoutSec()}), nil
}

// Cancel-all and the dead-man switch stop the account's triggers too when
// asked, and the cancel-all result counts the triggers it matched; without
// the option nothing changes.
func TestCancelAllCanIncludeTriggers(t *testing.T) {
	handler := &cancelAllHandler{}
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	orders := NewOrdersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	with, err := orders.CancelAllWith(ctx, nil, CancelAllParams{IncludeTriggers: true})
	if err != nil || with.MatchedTriggers != 3 {
		t.Fatalf("with triggers: %+v, %v", with, err)
	}
	without, err := orders.CancelAll(ctx, nil, nil, nil, nil, false, nil)
	if err != nil || without.MatchedTriggers != 0 {
		t.Fatalf("without triggers: %+v, %v", without, err)
	}
	if len(handler.cancelAll) != 2 || !handler.cancelAll[0].GetIncludeTriggers() || handler.cancelAll[1].GetIncludeTriggers() {
		t.Fatalf("cancel-all requests = %v", handler.cancelAll)
	}

	if _, err := orders.CancelAllAfterWith(ctx, nil, CancelAllAfterParams{TimeoutSec: 60, IncludeTriggers: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.CancelAllAfter(ctx, nil, 60, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(handler.cancelAllAfter) != 2 || !handler.cancelAllAfter[0].GetIncludeTriggers() || handler.cancelAllAfter[1].GetIncludeTriggers() || handler.cancelAllAfter[0].GetTimeoutSec() != 60 {
		t.Fatalf("cancel-all-after requests = %v", handler.cancelAllAfter)
	}
}
