package hardening_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// selectorServer serves a two-market catalog, BTC-USDT as 1 and LTC-USDT as 8,
// and records the symbol_id of each order history and cancel request.
type selectorServer struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	chainzipperv1connect.UnimplementedZipperServiceHandler
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	ordersv1connect.UnimplementedOrdersServiceHandler

	mu      sync.Mutex
	history [][]uint32
	cancels []uint32
}

func (s *selectorServer) GetSpotConfig(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
		Assets: []*marketdatav1.AssetConfig{
			{Asset: "BTC", QuantityScale: 8, MarketDataVolumeScale: 8},
			{Asset: "LTC", QuantityScale: 8, MarketDataVolumeScale: 8},
			{Asset: "USDT", QuantityScale: 6, MarketDataVolumeScale: 6},
		},
		Pairs: []*marketdatav1.PairConfig{
			{Symbol: "BTC-USDT", SymbolId: 1, BaseAsset: "BTC", QuoteAsset: "USDT", BaseQuantityScale: 8, QuoteQuantityScale: 6, ReferencePriceScale: 9},
			{Symbol: "LTC-USDT", SymbolId: 8, BaseAsset: "LTC", QuoteAsset: "USDT", BaseQuantityScale: 8, QuoteQuantityScale: 6, ReferencePriceScale: 9},
		},
	}), nil
}

func (s *selectorServer) GetDepositWithdrawConfig(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{}), nil
}

func (s *selectorServer) GetOrderHistory(_ context.Context, req *connect.Request[orderv1.GetOrderHistoryRequest]) (*connect.Response[orderv1.GetOrderHistoryResponse], error) {
	s.mu.Lock()
	s.history = append(s.history, slices.Clone(req.Msg.GetSymbolId()))
	s.mu.Unlock()
	return connect.NewResponse(&orderv1.GetOrderHistoryResponse{}), nil
}

func (s *selectorServer) CancelOrder(_ context.Context, req *connect.Request[orderv1.CancelOrderRequest]) (*connect.Response[orderv1.CancelOrderResponse], error) {
	s.mu.Lock()
	s.cancels = append(s.cancels, req.Msg.GetSymbolId())
	s.mu.Unlock()
	return connect.NewResponse(&orderv1.CancelOrderResponse{OrderId: 77, Status: orderv1.CancelOrderResponse_ACCEPTED}), nil
}

func (s *selectorServer) sent() (history [][]uint32, cancels []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.history), slices.Clone(s.cancels)
}

func newSelectorClient(t *testing.T, wire string) (*polyester.Client, *selectorServer) {
	t.Helper()
	server := &selectorServer{}
	mux := http.NewServeMux()
	mux.Handle(marketdatav1connect.NewMarketDataServiceHandler(server))
	mux.Handle(chainzipperv1connect.NewZipperServiceHandler(server))
	mux.Handle(ordersv1connect.NewOrdersReadServiceHandler(server))
	mux.Handle(ordersv1connect.NewOrdersServiceHandler(server))
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{
		APIURL:           srv.URL,
		APIKeyID:         "ak_selector_test",
		APIPrivateKey:    auth.NewSecret(auth.GenerateEd25519Keypair().SecretKeyHex.Reveal()),
		DefaultAccountID: new("7"),
		WireFormat:       wire,
		HydrateCatalogs:  true,
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client, server
}

type selectorCase struct {
	name     string
	symbol   *string
	symbolID *uint32
	// want is the symbol_id sent; 0 means the call is refused before sending.
	want uint32
}

func selectorCases() []selectorCase {
	btc, ltc, unknown := "BTC-USDT", "LTC-USDT", "DOGE-USDT"
	btcID := uint32(1)
	return []selectorCase{
		{"conflicting-name-and-id", &ltc, &btcID, 0},
		{"unknown-name-and-id", &unknown, &btcID, 0},
		{"matching-name-and-id", &btc, &btcID, 1},
		{"name-only", &ltc, nil, 8},
		{"id-only", nil, &btcID, 1},
		{"unknown-name-only", &unknown, nil, 0},
	}
}

func assertSelectorRefused(t *testing.T, err error) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T %v, want *errors.ValidationError", err, err)
	}
}

// Orders.ListHistory refuses a market name and a symbol_id that name
// different markets, or a name the catalog does not know paired with an id,
// before sending; matching and single selectors still send the right id.
func TestL2OrderHistoryRefusesConflictingSelectors(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		for _, tc := range selectorCases() {
			t.Run(wire+"/"+tc.name, func(t *testing.T) {
				client, server := newSelectorClient(t, wire)

				_, err := client.Orders.ListHistory(context.Background(), nil, nil, tc.symbol, tc.symbolID, nil, 3, false, false, nil)

				history, _ := server.sent()
				if tc.want == 0 {
					assertSelectorRefused(t, err)
					if len(history) != 0 {
						t.Fatalf("sent %d history request(s) with symbol_id %v", len(history), history)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(history) != 1 || !slices.Equal(history[0], []uint32{tc.want}) {
					t.Fatalf("history requests sent symbol_id %v, want [[%d]]", history, tc.want)
				}
			})
		}
	}
}

// Orders.Cancel applies the same rule as every other call that takes both
// selectors.
func TestL2OrderCancelResolvesSelectorsLikeOtherCalls(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		for _, tc := range selectorCases() {
			t.Run(wire+"/"+tc.name, func(t *testing.T) {
				client, server := newSelectorClient(t, wire)

				_, err := client.Orders.Cancel(context.Background(), nil, models.OrderKeyByID("77"), tc.symbol, tc.symbolID, nil)

				_, cancels := server.sent()
				if tc.want == 0 {
					assertSelectorRefused(t, err)
					if len(cancels) != 0 {
						t.Fatalf("sent %d cancel(s) with symbol_id %v", len(cancels), cancels)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(cancels, []uint32{tc.want}) {
					t.Fatalf("cancels sent symbol_id %v, want [%d]", cancels, tc.want)
				}
			})
		}
	}
}
