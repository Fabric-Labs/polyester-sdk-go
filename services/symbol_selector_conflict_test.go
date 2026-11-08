package services

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// twoPairCatalogs lists BTC-USDT as 5 and LTC-USDT as 6.
func twoPairCatalogs(t *testing.T) *catalogs.Manager {
	t.Helper()
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8)},
		map[string]any{"symbol": "LTC-USDT", "symbol_id": float64(6), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	return cats
}

func newTwoPairTriggerService(t *testing.T, handler *triggerControlHandler) *TriggersService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := triggersv1connect.NewTriggersServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), twoPairCatalogs(t), nil, nil, nil, nil, nil)
}

func TestTriggerControlRefusesANameAndIDOfDifferentMarkets(t *testing.T) {
	handler := &triggerControlHandler{}
	service := newTwoPairTriggerService(t, handler)
	triggerID := codecs.FormatID(9)
	bps := int32(50)
	ltc, btcID := "LTC-USDT", uint32(5)

	_, err := service.Modify(context.Background(), nil, triggerID, nil, &ltc, &btcID, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps})
	if err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("modify with LTC-USDT and symbol_id 5: %v", err)
	}
	_, err = service.Resume(context.Background(), nil, triggerID, nil, &ltc, &btcID)
	if err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("resume with LTC-USDT and symbol_id 5: %v", err)
	}
	if len(handler.modify) != 0 || len(handler.resumes) != 0 {
		t.Fatalf("sent %d modify and %d resume requests", len(handler.modify), len(handler.resumes))
	}
}

func TestTriggerControlSendsConsistentSelectors(t *testing.T) {
	ltc, btc := "LTC-USDT", "BTC-USDT"
	btcID := uint32(5)
	cases := []struct {
		name     string
		symbol   *string
		symbolID *uint32
		want     uint32
	}{
		{"name", &ltc, nil, 6},
		{"id", nil, &btcID, 5},
		{"matching", &btc, &btcID, 5},
		{"omitted", nil, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &triggerControlHandler{}
			service := newTwoPairTriggerService(t, handler)
			bps := int32(50)
			if _, err := service.Modify(context.Background(), nil, codecs.FormatID(9), nil, tc.symbol, tc.symbolID, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps}); err != nil {
				t.Fatalf("modify: %v", err)
			}
			if _, err := service.Resume(context.Background(), nil, codecs.FormatID(9), nil, tc.symbol, tc.symbolID); err != nil {
				t.Fatalf("resume: %v", err)
			}
			if got := handler.modify[0].GetSymbolId(); got != tc.want {
				t.Fatalf("modify symbol_id = %d, want %d", got, tc.want)
			}
			if got := handler.resumes[0].GetSymbolId(); got != tc.want {
				t.Fatalf("resume symbol_id = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestResolvingANameAndIDOfDifferentMarketsIsRefused(t *testing.T) {
	cats := twoPairCatalogs(t)
	ltc, doge := "LTC-USDT", "DOGE-USDT"
	btcID, ltcID, unknownID := uint32(5), uint32(6), uint32(7)
	if _, err := ResolveSymbolID(cats, &ltc, &btcID, "trades.list"); err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("LTC-USDT with symbol_id 5: %v", err)
	}
	if _, err := ResolveOptionalSymbolID(cats, &ltc, &btcID, "trades.list"); err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("optional LTC-USDT with symbol_id 5: %v", err)
	}
	if _, err := ResolveSymbolID(cats, &doge, &btcID, "trades.list"); err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("unknown name with symbol_id 5: %v", err)
	}
	for _, tc := range []struct {
		catalogs *catalogs.Manager
		symbol   *string
		id       *uint32
		want     uint32
	}{
		{cats, &ltc, &ltcID, 6},
		{cats, &doge, &unknownID, 7},
		{nil, &ltc, &btcID, 5},
	} {
		got, err := ResolveSymbolID(tc.catalogs, tc.symbol, tc.id, "trades.list")
		if err != nil || got != tc.want {
			t.Fatalf("ResolveSymbolID(%s, %d) = %d, %v; want %d", *tc.symbol, *tc.id, got, err, tc.want)
		}
	}
}
