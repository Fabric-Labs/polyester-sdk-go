package hardening_test

// POLY-9127: a method that names a market by symbol, or needs a pair's catalog
// scales, waits for construction-time catalog hydration.
//
// The stub server holds GetSpotConfig until the test releases it, so every
// call is made while hydration is still running. No timing is involved: a
// call that waits for hydration ends with its context's error when that is
// cancelled while hydration is held, where one that reads the empty catalog
// refuses the symbol without looking at its context.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	hydrationSymbol   = "DOGE-USDT"
	hydrationSymbolID = uint32(6)
)

func hydrationSpotConfig() *marketdatav1.GetSpotConfigResponse {
	return &marketdatav1.GetSpotConfigResponse{
		Assets: []*marketdatav1.AssetConfig{
			{Asset: "DOGE", QuantityScale: 8, MarketDataVolumeScale: 8},
			{Asset: "USDT", QuantityScale: 6, MarketDataVolumeScale: 6},
		},
		Pairs: []*marketdatav1.PairConfig{{
			Symbol:              hydrationSymbol,
			SymbolId:            hydrationSymbolID,
			BaseAsset:           "DOGE",
			QuoteAsset:          "USDT",
			BaseQuantityScale:   8,
			QuoteQuantityScale:  6,
			ReferencePriceScale: 9,
		}},
	}
}

// hydrationStub is a Connect server whose GetSpotConfig waits for release.
type hydrationStub struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	chainzipperv1connect.UnimplementedZipperServiceHandler

	spotRequested chan struct{}
	gate          chan struct{}
	once          sync.Once
	releaseOnce   sync.Once
	failSpot      bool

	mu        sync.Mutex
	reached   []string
	symbolIDs []uint32
}

func newHydrationStub() *hydrationStub {
	return &hydrationStub{spotRequested: make(chan struct{}), gate: make(chan struct{})}
}

func (h *hydrationStub) release(fail bool) {
	h.releaseOnce.Do(func() {
		h.failSpot = fail
		close(h.gate)
	})
}

func (h *hydrationStub) GetSpotConfig(ctx context.Context, _ *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	h.once.Do(func() { close(h.spotRequested) })
	select {
	case <-h.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if h.failSpot {
		return nil, connect.NewError(connect.CodeInternal, errors.New("spot down"))
	}
	return connect.NewResponse(hydrationSpotConfig()), nil
}

func (h *hydrationStub) GetDepositWithdrawConfig(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{}), nil
}

// record records any other RPC with the symbol_ids it carries and refuses it.
func (h *hydrationStub) record(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	ids := requestSymbolIDs(r.URL.Path, body)
	h.mu.Lock()
	h.reached = append(h.reached, r.URL.Path)
	h.symbolIDs = append(h.symbolIDs, ids...)
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	// Unimplemented: neither retried nor read as a validation refusal.
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"code":"unimplemented","message":"stub"}`))
}

func (h *hydrationStub) sent() ([]string, []uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.reached...), append([]uint32(nil), h.symbolIDs...)
}

// requestSymbolIDs decodes a Connect request by its procedure and returns
// every symbol_id set in it, nested messages included.
func requestSymbolIDs(path string, body []byte) []uint32 {
	procedure := strings.Trim(path, "/")
	service, method, ok := strings.Cut(procedure, "/")
	if !ok {
		return nil
	}
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil
	}
	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}
	methodDesc := serviceDesc.Methods().ByName(protoreflect.Name(method))
	if methodDesc == nil {
		return nil
	}
	msg := dynamicpb.NewMessage(methodDesc.Input())
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil
	}
	return collectSymbolIDs(msg)
}

func collectSymbolIDs(msg protoreflect.Message) []uint32 {
	var ids []uint32
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			for i := 0; i < v.List().Len(); i++ {
				ids = append(ids, collectSymbolIDs(v.List().Get(i).Message())...)
			}
		case fd.Message() != nil && !fd.IsMap():
			ids = append(ids, collectSymbolIDs(v.Message())...)
		case fd.Name() == "symbol_id" || fd.Name() == "symbol_ids":
			if fd.IsList() {
				for i := 0; i < v.List().Len(); i++ {
					ids = append(ids, uint32(v.List().Get(i).Uint()))
				}
			} else {
				ids = append(ids, uint32(v.Uint()))
			}
		}
		return true
	})
	return ids
}

func newHydrationClient(t *testing.T, stub *hydrationStub, hydrate bool) *polyester.Client {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(stub)
	mux.Handle(path+"GetSpotConfig", handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(stub)
	mux.Handle(zpath+"GetDepositWithdrawConfig", zhandler)
	mux.HandleFunc("/", stub.record)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	kp := auth.GenerateEd25519Keypair()
	accountID := "1"
	client, err := polyester.New(polyester.Config{
		APIURL:           srv.URL,
		WSURL:            "ws://127.0.0.1:1/unused",
		APIKeyID:         "ak_test",
		APIPrivateKey:    kp.SecretKeyHex,
		DefaultAccountID: &accountID,
		HydrateCatalogs:  hydrate,
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stub.release(false)
		_ = client.Close()
	})
	return client
}

type hydrationProbe struct {
	name string
	call func(ctx context.Context, c *polyester.Client, symbol string) error
	// filteredLocally: the request names no symbol_id; the SDK filters itself.
	filteredLocally bool
	// stream: the call ends at the realtime connection, after the symbol resolved.
	stream bool
}

func ptr[T any](v T) *T { return &v }

func limitOrder(symbol string) models.CreateOrderRequest {
	return models.CreateOrderRequest{
		Symbol:    &symbol,
		Side:      "buy",
		OrderType: "limit",
		TIF:       ptr("gtc"),
		Qty:       models.QtyFromDecimal("1"),
		Price:     ptr(models.PriceFromDecimal("0.1")),
	}
}

func hydrationProbes() []hydrationProbe {
	return []hydrationProbe{
		{name: "Orderbook.CreateSubscription", stream: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.Orderbook.CreateSubscription(ctx, services.CreateSubscriptionOptions{Symbol: s, Depth: 10})
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "Orderbook.Subscribe", stream: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.Orderbook.Subscribe(ctx, s, nil, 10)
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "Orderbook.Get", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orderbook.Get(ctx, s, 10)
			return err
		}},
		{name: "MarketData.GetTrades", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetTrades(ctx, &s, nil, 10, nil)
			return err
		}},
		{name: "MarketData.GetTradesAfter", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetTradesAfter(ctx, &s, nil, "0", 10)
			return err
		}},
		{name: "MarketData.GetCandles", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetCandles(ctx, &s, nil, "1m", 10, nil, nil, false)
			return err
		}},
		{name: "MarketData.GetCandlesWithReference", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetCandlesWithReference(ctx, &s, nil, "1m", 10, nil, nil, false, true)
			return err
		}},
		{name: "MarketData.GetCurrentCandle", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetCurrentCandle(ctx, &s, nil, "1m")
			return err
		}},
		{name: "MarketData.GetCandlesColumns", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketData.GetCandlesColumns(ctx, &s, nil, "1m", 10, nil, nil, false, nil)
			return err
		}},
		{name: "MarketData.SubscribeTrades", stream: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.MarketData.SubscribeTrades(ctx, &s, nil)
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "MarketData.SubscribeCandles", stream: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.MarketData.SubscribeCandles(ctx, &s, nil, "1m")
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "Heatmap.Get", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Heatmap.Get(ctx, &s, nil, "1s", 50, 10, "close", nil, nil, nil)
			return err
		}},
		{name: "Heatmap.SubscribeLive", stream: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.Heatmap.SubscribeLive(ctx, &s, nil, "1s")
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "MarketOverview.List", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketOverview.List(ctx, []string{s}, 10, "", false)
			return err
		}},
		{name: "MarketOverview.GetSpotVolumeHistory", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.MarketOverview.GetSpotVolumeHistory(ctx, []string{s}, nil)
			return err
		}},
		{name: "MarketOverview.CreateSubscription", filteredLocally: true, call: func(ctx context.Context, c *polyester.Client, s string) error {
			sub, err := c.MarketOverview.CreateSubscription(ctx, services.MarketOverviewCreateSubscriptionOptions{Symbols: []string{s}})
			if sub != nil {
				closeSub(sub)
			}
			return err
		}},
		{name: "Trades.List", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Trades.List(ctx, nil, nil, &s, nil, 10, nil, nil)
			return err
		}},
		{name: "Orders.ListHistory", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.ListHistory(ctx, nil, nil, &s, nil, nil, 10, false, false, nil)
			return err
		}},
		{name: "Orders.Cancel", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.Cancel(ctx, nil, models.OrderKeyByID("1"), &s, nil, nil)
			return err
		}},
		{name: "Orders.CancelAll", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.CancelAll(ctx, nil, nil, &s, nil, true, nil)
			return err
		}},
		{name: "Orders.CancelAllWith", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.CancelAllWith(ctx, nil, services.CancelAllParams{Symbols: []string{s}, DryRun: true})
			return err
		}},
		{name: "Orders.CancelAllAfter", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.CancelAllAfter(ctx, nil, 10, nil, &s, nil, nil)
			return err
		}},
		{name: "Orders.Create", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.Create(ctx, limitOrder(s), nil)
			return err
		}},
		{name: "Orders.Preview", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.Preview(ctx, limitOrder(s), nil)
			return err
		}},
		{name: "Orders.BatchCreate", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Orders.BatchCreate(ctx, nil, []models.CreateOrderRequest{limitOrder(s)}, nil, nil, nil, false)
			return err
		}},
		{name: "Orders.BatchReplace", call: func(ctx context.Context, c *polyester.Client, s string) error {
			qty := models.QtyFromDecimal("1")
			_, err := c.Orders.BatchReplace(ctx, nil, []models.BatchReplaceItem{{Key: models.OrderKeyByID("1"), NewQty: &qty}}, s, nil, nil)
			return err
		}},
		{name: "Orders.Modify", call: func(ctx context.Context, c *polyester.Client, s string) error {
			qty := models.QtyFromDecimal("1")
			_, err := c.Orders.Modify(ctx, nil, s, models.OrderKeyByID("1"), nil, nil, nil, &qty, nil, nil)
			return err
		}},
		{name: "Triggers.Create", call: func(ctx context.Context, c *polyester.Client, s string) error {
			price := models.PriceFromDecimal("0.1")
			_, err := c.Triggers.Create(ctx, nil, s, "stop_loss", &price, "sell", models.QtyFromDecimal("1"), "market", nil, "", "gtc", nil, nil, false, codecs.CreateTriggerOptions{})
			return err
		}},
		{name: "Triggers.List", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Triggers.List(ctx, nil, nil, &s, nil, 10, nil)
			return err
		}},
		{name: "Triggers.Resume", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Triggers.Resume(ctx, nil, "1", nil, &s, nil)
			return err
		}},
		{name: "Triggers.Modify", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.Triggers.Modify(ctx, nil, "1", nil, &s, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: ptr(int32(10))})
			return err
		}},
		{name: "Client.StopAllTrading", call: func(ctx context.Context, c *polyester.Client, s string) error {
			_, err := c.StopAllTrading(ctx, nil, services.StopAllTradingScope{Symbol: &s})
			return err
		}},
	}
}

func isSymbolRefusal(err error) bool {
	var validation *sdkerrors.ValidationError
	var hydration *sdkerrors.CatalogHydrationError
	return errors.As(err, &validation) || errors.As(err, &hydration)
}

// startHeld runs probe once hydration is held and returns its outcome channel.
func startHeld(ctx context.Context, t *testing.T, stub *hydrationStub, client *polyester.Client, probe hydrationProbe, symbol string) <-chan error {
	t.Helper()
	select {
	case <-stub.spotRequested:
	case <-time.After(10 * time.Second):
		t.Fatal("hydration never requested the spot catalog")
	}
	done := make(chan error, 1)
	go func() { done <- probe.call(ctx, client, symbol) }()
	return done
}

func awaitOutcome(t *testing.T, name string, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatalf("%s did not return", name)
		return nil
	}
}

func TestL2SymbolCallWaitsForInFlightHydration(t *testing.T) {
	for _, probe := range hydrationProbes() {
		t.Run(probe.name, func(t *testing.T) {
			stub := newHydrationStub()
			client := newHydrationClient(t, stub, true)

			// Cancelled while hydration is held: a call that waits returns its
			// context's error; one that reads the empty catalog refuses the symbol.
			ctx, cancel := context.WithCancel(context.Background())
			done := startHeld(ctx, t, stub, client, probe, hydrationSymbol)
			cancel()
			err := awaitOutcome(t, probe.name, done)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s did not wait for hydration: %T %v", probe.name, err, err)
			}

			// Once hydration ends, the call goes out with the catalog's symbol_id.
			done = startHeld(context.Background(), t, stub, client, probe, hydrationSymbol)
			stub.release(false)
			err = awaitOutcome(t, probe.name, done)
			if err == nil || isSymbolRefusal(err) {
				t.Fatalf("%s after hydration: want the stub's or stream's refusal, got %T %v", probe.name, err, err)
			}
			reached, ids := stub.sent()
			if probe.stream {
				if !strings.Contains(err.Error(), "127.0.0.1:1") && len(reached) == 0 {
					t.Fatalf("%s did not reach the stream: %v", probe.name, err)
				}
				return
			}
			if len(reached) == 0 {
				t.Fatalf("%s sent nothing: %v", probe.name, err)
			}
			if !probe.filteredLocally && !containsID(ids, hydrationSymbolID) {
				t.Fatalf("%s sent symbol_ids %v, want %d", probe.name, ids, hydrationSymbolID)
			}
		})
	}
}

func TestL2SymbolCallReportsFailedHydration(t *testing.T) {
	for _, probe := range hydrationProbes() {
		t.Run(probe.name, func(t *testing.T) {
			stub := newHydrationStub()
			client := newHydrationClient(t, stub, true)
			done := startHeld(context.Background(), t, stub, client, probe, hydrationSymbol)
			stub.release(true)
			err := awaitOutcome(t, probe.name, done)
			var hydration *sdkerrors.CatalogHydrationError
			if !errors.As(err, &hydration) {
				t.Fatalf("%s: want CatalogHydrationError, got %T %v", probe.name, err, err)
			}
			if !strings.Contains(err.Error(), "catalog hydration failed") || !strings.Contains(err.Error(), "spot down") {
				t.Fatalf("%s: error does not name the hydration failure: %v", probe.name, err)
			}
			if !errors.Is(err, client.CatalogsLastError()) {
				t.Fatalf("%s: error does not wrap the hydration failure %v", probe.name, client.CatalogsLastError())
			}
			if _, ids := stub.sent(); len(ids) != 0 {
				t.Fatalf("%s sent symbol_ids %v after a failed hydration", probe.name, ids)
			}
		})
	}
}

func TestL2UnknownSymbolAfterHydrationIsRefusedAsUnknown(t *testing.T) {
	for _, probe := range hydrationProbes() {
		t.Run(probe.name, func(t *testing.T) {
			stub := newHydrationStub()
			client := newHydrationClient(t, stub, true)
			stub.release(false)
			if err := client.WaitForCatalogs(context.Background()); err != nil {
				t.Fatal(err)
			}
			err := awaitOutcome(t, probe.name, startHeld(context.Background(), t, stub, client, probe, "NOPE-USDT"))
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(err.Error(), "Unknown symbol 'NOPE-USDT'") {
				t.Fatalf("%s: want the unknown-symbol ValidationError, got %T %v", probe.name, err, err)
			}
			if _, ids := stub.sent(); len(ids) != 0 {
				t.Fatalf("%s sent symbol_ids %v for an unknown symbol", probe.name, ids)
			}
		})
	}
}

func TestL2CancelledWaitDoesNotCancelHydration(t *testing.T) {
	stub := newHydrationStub()
	client := newHydrationClient(t, stub, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	<-stub.spotRequested
	go func() { done <- client.WaitForCatalogs(ctx) }()
	cancel()
	if err := awaitOutcome(t, "WaitForCatalogs", done); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	stub.release(false)
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatalf("hydration did not survive a cancelled wait: %v", err)
	}
	if id := client.Catalogs.SymbolIDForSymbol(hydrationSymbol); id == nil || *id != hydrationSymbolID {
		t.Fatalf("catalog lacks %s after hydration", hydrationSymbol)
	}
}

func TestL2HydrationDisabledKeepsTodaysRefusals(t *testing.T) {
	stub := newHydrationStub()
	client := newHydrationClient(t, stub, false)
	_, err := client.Orderbook.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: hydrationSymbol, Depth: 10})
	if err == nil || !strings.Contains(err.Error(), "needs the spot catalog for its symbol_id and quantity scale") {
		t.Fatalf("Orderbook.CreateSubscription: %v", err)
	}
	_, err = client.Orderbook.Get(context.Background(), hydrationSymbol, 10)
	if err == nil || !strings.Contains(err.Error(), "Unknown symbol 'DOGE-USDT'; set Config.HydrateCatalogs: true") {
		t.Fatalf("Orderbook.Get: %v", err)
	}
	select {
	case <-stub.spotRequested:
		t.Fatal("hydration ran with HydrateCatalogs off")
	default:
	}
}

func containsID(ids []uint32, want uint32) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// closeSub closes a subscription whichever Close signature it has.
func closeSub(sub any) {
	switch s := sub.(type) {
	case interface{ Close() error }:
		_ = s.Close()
	case interface{ Close() }:
		s.Close()
	}
}
