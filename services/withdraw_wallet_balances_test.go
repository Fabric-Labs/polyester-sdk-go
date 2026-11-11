package services

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1/chainwithdrawv1connect"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type walletBalancesHandler struct {
	chainwithdrawv1connect.UnimplementedWithdrawServiceHandler
	mu       sync.Mutex
	requests []*chainwithdrawv1.GetWalletBalancesRequest
	signed   []bool
}

func (h *walletBalancesHandler) GetWalletBalances(_ context.Context, req *connect.Request[chainwithdrawv1.GetWalletBalancesRequest]) (*connect.Response[chainwithdrawv1.GetWalletBalancesResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req.Msg)
	h.signed = append(h.signed, req.Header().Get("X-API-SIGNATURE") != "")
	return connect.NewResponse(&chainwithdrawv1.GetWalletBalancesResponse{
		SmartAccountAddress: "0x1111111111111111111111111111111111111111",
		Balances: []*chainwithdrawv1.WalletBalance{
			// 12.5 zTokens refunded by a rejected withdrawal.
			{ZippedAssetId: 3, UnifiedAssetId: 7, ChainId: 8453, ZtokenAddress: "0x2222222222222222222222222222222222222222", AmountE18: &typev1.U128{Lo: 12_500_000_000_000_000_000}},
			// More than 2^64 units: the amount is a full u128.
			{ZippedAssetId: 4, UnifiedAssetId: 8, ChainId: 1, ZtokenAddress: "0x3333333333333333333333333333333333333333", AmountE18: &typev1.U128{Hi: 1, Lo: 0}},
		},
	}), nil
}

func newWalletBalancesService(t *testing.T, handler *walletBalancesHandler, defaultSub *string) *WithdrawService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := chainwithdrawv1connect.NewWithdrawServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewWithdrawService(transport.NewFactory(cfg, creds, nil), defaultSub)
}

func TestGetWalletBalancesDecodesEveryZTokenBalance(t *testing.T) {
	handler := &walletBalancesHandler{}
	service := newWalletBalancesService(t, handler, nil)

	got, err := service.GetWalletBalances(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := models.WalletBalances{
		SmartAccountAddress: "0x1111111111111111111111111111111111111111",
		Balances: []models.WalletBalance{
			{ZippedAssetID: 3, UnifiedAssetID: 7, ChainID: 8453, ZTokenAddress: "0x2222222222222222222222222222222222222222", AmountE18: "12500000000000000000", Amount: "12.5"},
			{ZippedAssetID: 4, UnifiedAssetID: 8, ChainID: 1, ZTokenAddress: "0x3333333333333333333333333333333333333333", AmountE18: "18446744073709551616", Amount: "18.446744073709551616"},
		},
	}
	if got.SmartAccountAddress != want.SmartAccountAddress || len(got.Balances) != len(want.Balances) {
		t.Fatalf("got %+v", got)
	}
	for i := range want.Balances {
		if got.Balances[i] != want.Balances[i] {
			t.Fatalf("balance %d = %+v, want %+v", i, got.Balances[i], want.Balances[i])
		}
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.requests) != 1 || handler.requests[0].GetSubaccountId() != 0 || !handler.signed[0] {
		t.Fatalf("requests=%v signed=%v; want one signed main-account read", handler.requests, handler.signed)
	}
}

func TestGetWalletBalancesTargetsTheSubAccount(t *testing.T) {
	handler := &walletBalancesHandler{}
	defaultSub := codecs.FormatID(41)
	service := newWalletBalancesService(t, handler, &defaultSub)
	explicit := codecs.FormatID(42)

	if _, err := service.GetWalletBalances(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetWalletBalances(context.Background(), nil, &explicit); err != nil {
		t.Fatal(err)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.requests) != 2 || handler.requests[0].GetSubaccountId() != 41 || handler.requests[1].GetSubaccountId() != 42 {
		t.Fatalf("requests=%v", handler.requests)
	}
}
