package services

import (
	"context"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	ledger "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	market "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	overview "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1/marketoverviewv1connect"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type readPreservationHandler struct {
	ledgerrdv1connect.UnimplementedLedgerReadServiceHandler
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	marketoverviewv1connect.UnimplementedMarketOverviewServiceHandler
	ordersv1connect.UnimplementedOrdersServiceHandler
	mu    sync.Mutex
	calls []proto.Message
}

func (h *readPreservationHandler) record(msg proto.Message) (string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, proto.Clone(msg))
	if len(h.calls) == 1 {
		return "eyJ2IjoxfQ==", 1
	}
	return "", len(h.calls)
}
func (h *readPreservationHandler) ListTransfers(_ context.Context, r *connect.Request[ledger.ListTransfersRequest]) (*connect.Response[ledger.ListTransfersResponse], error) {
	token, n := h.record(r.Msg)
	return connect.NewResponse(&ledger.ListTransfersResponse{Transfers: []*ledger.TransferRow{{TsUs: 999, TransferId: codecs.FormatID(uint64(n))}}, NextPageToken: token}), nil
}
func (h *readPreservationHandler) ListMarketOverview(_ context.Context, r *connect.Request[overview.ListMarketOverviewRequest]) (*connect.Response[overview.ListMarketOverviewResponse], error) {
	token, _ := h.record(r.Msg)
	return connect.NewResponse(&overview.ListMarketOverviewResponse{Markets: []*overview.MarketOverview{{SymbolId: 1}}, NextPageToken: token}), nil
}
func (h *readPreservationHandler) GetCandles(_ context.Context, r *connect.Request[market.GetCandlesRequest]) (*connect.Response[market.GetCandlesResponse], error) {
	token, _ := h.record(r.Msg)
	return connect.NewResponse(&market.GetCandlesResponse{Candles: []*market.CandlePoint{{TsSec: 100}}, NextPageToken: token}), nil
}
func (h *readPreservationHandler) PreviewOrder(_ context.Context, r *connect.Request[orders.PreviewOrderRequest]) (*connect.Response[orders.PreviewOrderResponse], error) {
	h.record(r.Msg)
	qty := int64(16000)
	return connect.NewResponse(&orders.PreviewOrderResponse{ResolvedBaseQtyScaled: &qty, EvaluatedAt: &timestamppb.Timestamp{Seconds: 1}}), nil
}
func newReadPreservationServices(t *testing.T, h *readPreservationHandler, wire string) (*transport.Factory, *catalogs.Manager) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := ledgerrdv1connect.NewLedgerReadServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = marketdatav1connect.NewMarketDataServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = marketoverviewv1connect.NewMarketOverviewServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = ordersv1connect.NewOrdersServiceHandler(h)
	mux.Handle(path, handler)
	server := hardening.NewTestServer(mux)
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	cats := catalogs.NewManager()
	if err = cats.HydrateSpotConfig(map[string]any{"assets": []any{map[string]any{"asset": "BTC", "market_data_volume_scale": float64(8)}}, "pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_asset": "BTC", "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)}}}); err != nil {
		t.Fatal(err)
	}
	return transport.NewFactory(cfg, creds, nil), cats
}
func TestReadPublicOpaquePaging(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		for _, operation := range []string{"transfers", "overview", "candles"} {
			t.Run(wire+operation, func(t *testing.T) {
				h := &readPreservationHandler{}
				factory, cats := newReadPreservationServices(t, h, wire)
				ctx := context.Background()
				var firstToken, secondToken string
				switch operation {
				case "transfers":
					service := NewTransfersService(factory, nil, nil, nil)
					sub := codecs.FormatID(42)
					since := int64(100)
					first, err := service.List(ctx, nil, &sub, 1, true, &since)
					if err != nil {
						t.Fatal(err)
					}
					firstToken = first.NextPageToken
					second, err := service.List(ctx, nil, &sub, 1, true, &since, firstToken)
					if err != nil {
						t.Fatal(err)
					}
					secondToken = second.NextPageToken
					if len(first.Transfers) != 1 || len(second.Transfers) != 1 || first.Transfers[0].Timestamp != 999 || second.Transfers[0].Timestamp != 999 || first.Transfers[0].TransferID == second.Transfers[0].TransferID {
						t.Fatal("equal-timestamp scenario missing")
					}
				case "overview":
					service := NewMarketOverviewService(factory, cats, nil)
					first, err := service.List(ctx, []string{"BTC-USDT"}, 1, "", false)
					if err != nil {
						t.Fatal(err)
					}
					firstToken = first.NextPageToken
					second, err := service.List(ctx, []string{"BTC-USDT"}, 1, "", false, firstToken)
					if err != nil {
						t.Fatal(err)
					}
					secondToken = second.NextPageToken
					if len(first.Markets) != 1 || len(second.Markets) != 1 {
						t.Fatal("missing markets")
					}
				case "candles":
					service := NewMarketDataService(factory, cats, nil)
					id := uint32(1)
					first, err := service.GetCandles(ctx, nil, &id, "5m", 1, nil, nil, false)
					if err != nil {
						t.Fatal(err)
					}
					firstToken = first.NextPageToken
					second, err := service.GetCandles(ctx, nil, &id, "5m", 1, nil, nil, false, firstToken)
					if err != nil {
						t.Fatal(err)
					}
					secondToken = second.NextPageToken
					if len(first.Candles) != 1 || len(second.Candles) != 1 {
						t.Fatal("missing candles")
					}
				}
				if firstToken != "eyJ2IjoxfQ==" || secondToken != "" {
					t.Fatalf("tokens %q %q", firstToken, secondToken)
				}
				h.mu.Lock()
				defer h.mu.Unlock()
				if len(h.calls) != 2 {
					t.Fatalf("calls=%d", len(h.calls))
				}
				switch msg := h.calls[1].(type) {
				case *ledger.ListTransfersRequest:
					if msg.PageToken != firstToken || msg.GetSubaccountId() != 42 || msg.TsMinUs != 100 {
						t.Fatal(msg)
					}
					msg.PageToken = ""
				case *overview.ListMarketOverviewRequest:
					if msg.PageToken != firstToken {
						t.Fatal(msg)
					}
					msg.PageToken = ""
				case *market.GetCandlesRequest:
					if msg.PageToken != firstToken {
						t.Fatal(msg)
					}
					msg.PageToken = ""
				}
				if !proto.Equal(h.calls[0], h.calls[1]) {
					t.Fatal("filters changed")
				}
			})
		}
	}
}
func TestReadPreviewSymbolIDUsesCatalogScale(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &readPreservationHandler{}
			factory, cats := newReadPreservationServices(t, h, wire)
			service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
			id := uint32(1)
			price := models.PriceFromDecimal("100")
			result, err := service.Preview(context.Background(), models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "limit", Qty: models.QtyFromScaledInt(16000), Price: &price}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.ResolvedBaseQty == nil {
				t.Fatal("missing resolved size")
			}
			formatted, err := result.ResolvedBaseQty.Format()
			if err != nil {
				t.Fatal(err)
			}
			if formatted != "0.00016" || result.ResolvedBaseQtyScaled != "16000" {
				t.Fatal(result)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.calls) != 1 {
				t.Fatalf("calls=%d", len(h.calls))
			}
		})
	}
}

func (h *readPreservationHandler) GetEquityHistorySeries(_ context.Context, r *connect.Request[ledger.GetEquityHistorySeriesRequest]) (*connect.Response[ledger.GetEquityHistorySeriesResponse], error) {
	_, n := h.record(r.Msg)
	var prices []int64
	if n == 1 {
		prices = []int64{60000000000001, 61000000000009}
	}
	return connect.NewResponse(&ledger.GetEquityHistorySeriesResponse{Points: 2, StartTsSec: 100, EndTsSec: 400, Bucket: "5m", QuoteAsset: "USDT", Series: []*ledger.EquitySeries{{EquityQ: []int64{12345, -6789}}}, BtcPricesQ: prices}), nil
}
func TestReadPublicEquityBTCPricesAndOmittedControl(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &readPreservationHandler{}
			factory, cats := newReadPreservationServices(t, h, wire)
			service := NewBalancesService(factory, cats, nil, nil, nil)
			for _, prices := range [][]int64{{60000000000001, 61000000000009}, {}} {
				result, err := service.GetEquityHistory(context.Background(), nil, "1d", nil, nil, "account")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(result.BTCPricesQ, prices) {
					t.Fatalf("got %v want %v", result.BTCPricesQ, prices)
				}
				if result.Points != 2 || result.StartTsSec != 100 || result.EndTsSec != 400 || len(result.Series) != 1 || !reflect.DeepEqual(result.Series[0].EquityQ, []int64{12345, -6789}) {
					t.Fatal(result)
				}
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.calls) != 2 {
				t.Fatalf("reads did not occur: %d", len(h.calls))
			}
			if !proto.Equal(h.calls[0], h.calls[1]) {
				t.Fatal("control request changed")
			}
		})
	}
}
