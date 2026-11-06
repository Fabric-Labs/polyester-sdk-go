package services

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type subaccountViewHandler struct {
	authv1connect.UnimplementedSubaccountViewServiceHandler
	mu       sync.Mutex
	requests []*authv1.GetSubaccountRequest
}

// GetSubaccount answers balances only when they were asked for, as the
// server does.
func (h *subaccountViewHandler) GetSubaccount(_ context.Context, req *connect.Request[authv1.GetSubaccountRequest]) (*connect.Response[authv1.GetSubaccountResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req.Msg)
	resp := &authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: req.Msg.GetSubaccountId()}}
	if req.Msg.GetIncludeBalances() {
		resp.Balances = &ledgerrdv1.GetBalancesResponse{Balances: []*ledgerrdv1.AssetBalance{{
			AssetId: 1, Trading: &typev1.U128{Lo: 7}, Available: &typev1.U128{Lo: 7},
		}}}
	}
	return connect.NewResponse(resp), nil
}

func TestSubAccountsGetRequestsBalancesOnlyWhenAsked(t *testing.T) {
	handler := &subaccountViewHandler{}
	mux := http.NewServeMux()
	path, h := authv1connect.NewSubaccountViewServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	service := NewSubAccountsService(transport.NewFactory(cfg, creds, nil), nil, nil, nil)
	subID := codecs.FormatID(42)

	plain, err := service.Get(context.Background(), nil, &subID, false, false, false, false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Balances != nil {
		t.Fatalf("balances without includeBalances=%+v", plain.Balances)
	}
	withBalances, err := service.Get(context.Background(), nil, &subID, false, false, false, false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if withBalances.Balances == nil || len(withBalances.Balances.Balances) != 1 ||
		withBalances.Balances.Balances[0].AssetID != 1 || withBalances.Balances.Balances[0].Available != "7" {
		t.Fatalf("balances=%+v", withBalances.Balances)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.requests) != 2 || handler.requests[0].GetIncludeBalances() || !handler.requests[1].GetIncludeBalances() ||
		handler.requests[1].GetSubaccountId() != 42 {
		t.Fatalf("requests=%v", handler.requests)
	}
}
