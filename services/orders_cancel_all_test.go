package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// errCatalogHydration is the hydration failure of a failing catalog fixture.
var errCatalogHydration = errors.New("spot config unavailable")

// catalogFixture is how the services' catalogs behave: loaded, hydration
// failed, or hydration still pending.
type catalogFixture int

const (
	catalogsLoaded catalogFixture = iota
	catalogsFailed
	catalogsPending
)

// newCancelAllOrdersServices serves cancelAllHandler for orders and an
// unimplemented triggers service, counting every request the server reads.
func newCancelAllOrdersServices(t *testing.T, handler *cancelAllHandler) (*OrdersService, *TriggersService, *atomic.Int64) {
	t.Helper()
	return newCancelAllOrdersServicesWith(t, handler, catalogsLoaded)
}

func newCancelAllOrdersServicesWith(t *testing.T, handler *cancelAllHandler, fixture catalogFixture) (*OrdersService, *TriggersService, *atomic.Int64) {
	t.Helper()
	var sent atomic.Int64
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersServiceHandler(handler)
	mux.Handle(path, h)
	path, h = triggersv1connect.NewTriggersServiceHandler(triggersv1connect.UnimplementedTriggersServiceHandler{})
	mux.Handle(path, h)
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	var done chan struct{}
	var lastError func() error
	switch fixture {
	case catalogsLoaded:
		if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
			"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
		}}}); err != nil {
			t.Fatal(err)
		}
	case catalogsFailed:
		done = make(chan struct{})
		close(done)
		lastError = func() error { return errCatalogHydration }
	case catalogsPending:
		done = make(chan struct{})
	}
	factory := transport.NewFactory(cfg, creds, nil)
	orders := NewOrdersService(factory, cats, nil, nil, nil, done, lastError)
	return orders, NewTriggersService(factory, cats, nil, nil, nil, done, lastError), &sent
}

// A blank symbol filter, or a request ID the SDK would have to trim or
// replace, is refused before anything is sent: it must never reach the
// server as a cancel of every pair, or as a retry with another key.
func TestOrdersCancelAllRefusesBlankFiltersBeforeSending(t *testing.T) {
	blank := "  "
	empty := ""
	padded := " retry-1 "
	symbol := "BTC-USDT"
	cases := map[string]CancelAllParams{
		"blank symbol":            {Symbol: &blank},
		"empty symbol":            {Symbol: &empty, DryRun: true},
		"empty entry in symbols":  {Symbols: []string{""}},
		"blank entry in symbols":  {Symbols: []string{"BTC-USDT", " \t"}},
		"blank symbol with ids":   {Symbol: &blank, SymbolIDs: []uint32{5}},
		"symbol and symbol ids":   {Symbol: &symbol, SymbolIDs: []uint32{5}},
		"symbols and symbol ids":  {Symbols: []string{"BTC-USDT"}, SymbolIDs: []uint32{5}},
		"symbol and symbols":      {Symbol: &symbol, Symbols: []string{"BTC-USDT"}},
		"empty request id":        {RequestID: &empty},
		"blank request id":        {RequestID: &blank},
		"padded request id":       {RequestID: &padded},
		"bad request id":          {RequestID: ptrString("bad!key")},
		"long request id":         {RequestID: ptrString(strings.Repeat("r", 65))},
		"blank symbol, valid key": {Symbol: &blank, RequestID: ptrString("retry-1")},
		"symbol, bad request id":  {Symbol: &symbol, RequestID: ptrString("bad!key")},
	}
	// Refused before the catalogs are awaited: a failed hydration, or one
	// still pending, never decides the answer, and nothing is sent.
	for fixtureName, fixture := range map[string]catalogFixture{"failed catalogs": catalogsFailed, "pending catalogs": catalogsPending} {
		for name, params := range cases {
			t.Run(fixtureName+"/"+name, func(t *testing.T) {
				handler := &cancelAllHandler{}
				orders, _, sent := newCancelAllOrdersServicesWith(t, handler, fixture)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := orders.CancelAllWith(ctx, nil, params)
				var validation *sdkerrors.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("err = %v, want a ValidationError", err)
				}
				if n := sent.Load(); n != 0 || len(handler.cancelAll) != 0 {
					t.Fatalf("sent %d requests (%d cancel-alls)", n, len(handler.cancelAll))
				}
			})
		}
	}

	// The positional form refuses a blank symbol the same way.
	handler := &cancelAllHandler{}
	orders, _, sent := newCancelAllOrdersServices(t, handler)
	_, err := orders.CancelAll(context.Background(), nil, nil, &blank, nil, true, nil)
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "symbol must not be blank") {
		t.Fatalf("CancelAll blank symbol: err = %v", err)
	}
	if sent.Load() != 0 {
		t.Fatalf("CancelAll blank symbol sent %d requests", sent.Load())
	}
}

// Leaving every symbol filter unset still cancels on every pair, and a
// request ID the caller supplies is sent exactly as given.
func TestOrdersCancelAllWithoutFiltersStillSends(t *testing.T) {
	handler := &cancelAllHandler{}
	orders, _, _ := newCancelAllOrdersServices(t, handler)
	ctx := context.Background()
	requestID := "ui/cancel-all:7f3a"
	if _, err := orders.CancelAll(ctx, nil, nil, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.CancelAllWith(ctx, nil, CancelAllParams{Symbols: []string{}, SymbolIDs: []uint32{}, RequestID: &requestID}); err != nil {
		t.Fatal(err)
	}
	if len(handler.cancelAll) != 2 {
		t.Fatalf("sent %d cancel-alls, want 2", len(handler.cancelAll))
	}
	for i, req := range handler.cancelAll {
		if len(req.GetSymbolIds()) != 0 {
			t.Fatalf("request %d symbol_ids = %v, want every pair", i, req.GetSymbolIds())
		}
	}
	if got := handler.cancelAll[0].GetRequestId(); !strings.HasPrefix(got, "cancel-all-") {
		t.Fatalf("generated request_id = %q", got)
	}
	if got := handler.cancelAll[1].GetRequestId(); got != requestID {
		t.Fatalf("request_id = %q, want %q", got, requestID)
	}
}

// StopAllTrading refuses a blank symbol, or a request ID its cancel-all
// would refuse, before it lists or cancels any trigger.
func TestStopAllTradingRefusesBlankFiltersBeforeSending(t *testing.T) {
	blank := " "
	padded := "stop-1 "
	for name, scope := range map[string]StopAllTradingScope{
		"blank symbol":      {Symbol: &blank},
		"padded request id": {RequestID: &padded},
		"empty request id":  {RequestID: ptrString("")},
		"bad request id":    {RequestID: ptrString("bad!key")},
		"long request id":   {RequestID: ptrString(strings.Repeat("r", 65))},
	} {
		t.Run(name, func(t *testing.T) {
			handler := &cancelAllHandler{}
			orders, triggers, sent := newCancelAllOrdersServicesWith(t, handler, catalogsFailed)
			_, err := StopAllTrading(context.Background(), triggers, orders, nil, scope)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
			if sent.Load() != 0 {
				t.Fatalf("sent %d requests", sent.Load())
			}
		})
	}
}

func ptrString(value string) *string { return &value }
