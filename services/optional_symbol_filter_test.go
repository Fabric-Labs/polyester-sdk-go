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
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// pendingCatalogServices builds the services that take an optional market
// filter against a server that counts every request it reads, with a catalog
// load that never finishes: a call that waited for the catalogs would time
// out instead of returning a ValidationError.
type pendingCatalogServices struct {
	orders   *OrdersService
	triggers *TriggersService
	pnl      *ProfitAndLossService
	alerts   *AlertsService
	sent     *atomic.Int64
}

func newPendingCatalogServices(t *testing.T) pendingCatalogServices {
	t.Helper()
	var sent atomic.Int64
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sent.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	_ = cats.BeginHydration() // never finished
	done := make(chan struct{})
	lastError := func() error { return nil }
	factory := transport.NewFactory(cfg, creds, nil)
	return pendingCatalogServices{
		orders:   NewOrdersService(factory, cats, nil, nil, nil, done, lastError),
		triggers: NewTriggersService(factory, cats, nil, nil, nil, done, lastError),
		pnl:      NewProfitAndLossService(factory, cats, nil),
		alerts:   NewAlertsService(factory, cats),
		sent:     &sent,
	}
}

// Every call taking an optional market filter refuses a supplied but blank
// market name with a ValidationError, before any catalog wait or request: a
// blank name must never widen the call to every market. Above all the
// dead-man switch, which with IncludeTriggers would also stop every trigger
// on every pair when it fires.
func TestOptionalMarketFilterRefusesBlankSymbolBeforeSending(t *testing.T) {
	symbolID := uint32(5)
	triggerID := codecs.FormatID(9)
	bps := int32(50)
	calls := map[string]func(ctx context.Context, s pendingCatalogServices, symbol *string) error{
		"orders.CancelAllAfter": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.orders.CancelAllAfter(ctx, nil, 30, nil, symbol, nil, nil)
			return err
		},
		"orders.CancelAllAfterWith include triggers": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.orders.CancelAllAfterWith(ctx, nil, CancelAllAfterParams{TimeoutSec: 30, Symbol: symbol, IncludeTriggers: true})
			return err
		},
		"pnl.Get": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.pnl.Get(ctx, ProfitAndLossParams{Window: "days_7", Symbol: symbol})
			return err
		},
		"pnl.Get with symbol id": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.pnl.Get(ctx, ProfitAndLossParams{Window: "days_7", Symbol: symbol, SymbolID: &symbolID})
			return err
		},
		"pnl.GetTradingStatistics": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.pnl.GetTradingStatistics(ctx, ProfitAndLossParams{Window: "days_7", Symbol: symbol})
			return err
		},
		"alerts.List": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.alerts.List(ctx, ListAlertsParams{Symbol: symbol})
			return err
		},
		"triggers.List": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.triggers.List(ctx, nil, nil, symbol, nil, 0, nil)
			return err
		},
		"triggers.Resume": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.triggers.Resume(ctx, nil, triggerID, nil, symbol, nil)
			return err
		},
		"triggers.Modify": func(ctx context.Context, s pendingCatalogServices, symbol *string) error {
			_, err := s.triggers.Modify(ctx, nil, triggerID, nil, symbol, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps})
			return err
		},
	}
	for name, call := range calls {
		for _, blank := range []string{"", "   ", "\t"} {
			t.Run(name+"/"+strings.ReplaceAll(blank, "\t", `\t`), func(t *testing.T) {
				s := newPendingCatalogServices(t)
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				symbol := blank
				err := call(ctx, s, &symbol)
				var validation *sdkerrors.ValidationError
				if !errors.As(err, &validation) || !strings.Contains(err.Error(), "must not be blank") {
					t.Fatalf("err = %v, want a blank-symbol ValidationError", err)
				}
				if n := s.sent.Load(); n != 0 {
					t.Fatalf("sent %d requests", n)
				}
			})
		}
	}
}

// Omitting the market still arms the dead-man switch for every market: no
// symbol_id is sent.
func TestCancelAllAfterWithoutSymbolArmsEveryMarket(t *testing.T) {
	handler := &cancelAllHandler{}
	orders, _, _ := newCancelAllOrdersServices(t, handler)
	ctx := context.Background()
	if _, err := orders.CancelAllAfter(ctx, nil, 30, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.CancelAllAfterWith(ctx, nil, CancelAllAfterParams{TimeoutSec: 30, IncludeTriggers: true}); err != nil {
		t.Fatal(err)
	}
	symbol := " BTC-USDT "
	if _, err := orders.CancelAllAfterWith(ctx, nil, CancelAllAfterParams{TimeoutSec: 30, Symbol: &symbol}); err != nil {
		t.Fatal(err)
	}
	if len(handler.cancelAllAfter) != 3 {
		t.Fatalf("sent %d cancel-all-afters, want 3", len(handler.cancelAllAfter))
	}
	for i, req := range handler.cancelAllAfter[:2] {
		if req.GetSymbolId() != 0 {
			t.Fatalf("request %d symbol_id = %d, want every market", i, req.GetSymbolId())
		}
	}
	if !handler.cancelAllAfter[1].GetIncludeTriggers() {
		t.Fatal("include_triggers not sent")
	}
	if got := handler.cancelAllAfter[2].GetSymbolId(); got != 5 {
		t.Fatalf("named market symbol_id = %d, want 5", got)
	}
}
