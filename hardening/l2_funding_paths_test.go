package hardening_test

// POLY-9228: only what reads the funding (deposit/withdraw) config waits for
// it. The zipper supply stream sends nothing while the config is pending,
// and returns the funding config's *errors.CatalogHydrationError without
// sending once it failed. Deposit addresses and withdrawals read nothing
// from it and keep working.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

type fundingState string

const (
	fundingPending fundingState = "pending"
	fundingFailed  fundingState = "failed"
)

type fundingHarness struct {
	client     *polyester.Client
	wsConnects *atomic.Int64
	mu         sync.Mutex
	sent       []string
}

func (h *fundingHarness) last() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sent) == 0 {
		return ""
	}
	return h.sent[len(h.sent)-1]
}

func newFundingHarness(t *testing.T, state fundingState) *fundingHarness {
	t.Helper()
	h := &fundingHarness{wsConnects: &atomic.Int64{}}
	srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		switch {
		case strings.HasSuffix(req.Path, "/GetSpotConfig"):
			return startupSpotResponse(t)
		case strings.HasSuffix(req.Path, "/GetDepositWithdrawConfig"):
			if state == fundingPending {
				return hardening.NeverRespond()
			}
			return hardening.Json(http.StatusInternalServerError, []byte(`{"code":"internal","message":"funding down"}`))
		}
		h.mu.Lock()
		h.sent = append(h.sent, req.Path[strings.LastIndex(req.Path, "/")+1:])
		h.mu.Unlock()
		return hardening.Json(http.StatusBadRequest, []byte(`{"code":"failed_precondition","message":"stub"}`))
	})
	t.Cleanup(srv.Close)
	ws, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	go func() {
		for {
			conn, err := ws.Accept()
			if err != nil {
				return
			}
			h.wsConnects.Add(1)
			_ = conn.Close()
		}
	}()
	kp := auth.GenerateEd25519Keypair()
	accountID := "1"
	client, err := polyester.New(polyester.Config{
		APIURL:           srv.BaseURL(),
		WSURL:            "ws://" + ws.Addr().String() + "/connection/websocket",
		APIKeyID:         "ak_test",
		APIPrivateKey:    kp.SecretKeyHex,
		DefaultAccountID: &accountID,
		HydrateCatalogs:  true,
		Timeout:          30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state == fundingFailed {
		if err := client.WaitForFundingCatalog(context.Background()); err == nil {
			t.Fatal("the funding config loaded")
		}
	}
	h.client = client
	return h
}

func TestL2SupplyStreamSendsNothingWithoutTheFundingConfig(t *testing.T) {
	for _, state := range []fundingState{fundingPending, fundingFailed} {
		t.Run(string(state), func(t *testing.T) {
			h := newFundingHarness(t, state)
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			_, err := h.client.Zipper.SubscribeZippedAssetSupply(ctx, true)
			var hydrationErr *sdkerrors.CatalogHydrationError
			switch state {
			case fundingPending:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("pending: %T %v, want the caller's deadline", err, err)
				}
			case fundingFailed:
				if !errors.As(err, &hydrationErr) || hydrationErr.Catalog != "funding" {
					t.Fatalf("failed: %T %v, want the funding config's failure", err, err)
				}
			}
			if n := h.wsConnects.Load(); n != 0 {
				t.Fatalf("connected to the stream %d times without the funding config", n)
			}
			// A lookup that reads the funding config answers nil, not a guess.
			if h.client.Catalogs.LedgerIDForAsset("USDT") != nil {
				t.Fatal("asset lookup answered without the funding config")
			}
		})
	}
}

func TestL2DepositAddressesAndWithdrawalsDoNotWaitForTheFundingConfig(t *testing.T) {
	deadline := uint64(4_102_444_800)
	for _, state := range []fundingState{fundingPending, fundingFailed} {
		t.Run(string(state), func(t *testing.T) {
			h := newFundingHarness(t, state)
			calls := []struct {
				operation string
				call      func(context.Context) error
			}{
				{"ListDepositAddresses", func(ctx context.Context) error {
					_, err := h.client.Deposit.ListAddresses(ctx, 1, nil, nil)
					return err
				}},
				{"CreateDepositAddress", func(ctx context.Context) error {
					_, err := h.client.Deposit.CreateAddress(ctx, 1, nil, nil)
					return err
				}},
				{"CreateTradingWithdraw", func(ctx context.Context) error {
					_, err := h.client.Withdraw.CreateToFunding(ctx, 2, models.AssetAmountFromDecimal("1"),
						make([]byte, 65), "poly-9228-control", "1", &deadline, nil, nil, "", 6)
					return err
				}},
			}
			for _, c := range calls {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := c.call(ctx)
				cancel()
				var hydrationErr *sdkerrors.CatalogHydrationError
				if err == nil || errors.As(err, &hydrationErr) {
					t.Fatalf("%s: %v, want the stub's refusal", c.operation, err)
				}
				if got := h.last(); got != c.operation {
					t.Fatalf("%s was not sent (last request %q): %v", c.operation, got, err)
				}
			}
		})
	}
}
