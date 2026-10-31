package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type triggerControlHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	mu      sync.Mutex
	modify  []*triggersv1.ModifyTriggerRequest
	resumes []*triggersv1.ResumeTriggerRequest
}

func (h *triggerControlHandler) ModifyTrigger(_ context.Context, req *connect.Request[triggersv1.ModifyTriggerRequest]) (*connect.Response[triggersv1.ModifyTriggerResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.modify = append(h.modify, req.Msg)
	return connect.NewResponse(&triggersv1.ModifyTriggerResponse{TriggerId: req.Msg.GetTriggerId()}), nil
}

func (h *triggerControlHandler) ResumeTrigger(_ context.Context, req *connect.Request[triggersv1.ResumeTriggerRequest]) (*connect.Response[triggersv1.ResumeTriggerResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resumes = append(h.resumes, req.Msg)
	return connect.NewResponse(&triggersv1.ResumeTriggerResponse{TriggerId: req.Msg.GetTriggerId()}), nil
}

func newTriggerControlService(t *testing.T, handler *triggerControlHandler) *TriggersService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := triggersv1connect.NewTriggersServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

func TestTriggerModifyAndResumeOmitSymbolWhenNotGiven(t *testing.T) {
	handler := &triggerControlHandler{}
	service := newTriggerControlService(t, handler)
	triggerID := codecs.FormatID(9)
	bps := int32(50)

	if _, err := service.Modify(context.Background(), nil, triggerID, nil, nil, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps}); err != nil {
		t.Fatalf("modify without symbol: %v", err)
	}
	if _, err := service.Resume(context.Background(), nil, triggerID, nil, nil, nil); err != nil {
		t.Fatalf("resume without symbol: %v", err)
	}
	// A symbol, when given, still targets that market.
	symbol := "BTC-USDT"
	if _, err := service.Modify(context.Background(), nil, triggerID, nil, &symbol, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(context.Background(), nil, triggerID, nil, &symbol, nil); err != nil {
		t.Fatal(err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.modify) != 2 || handler.modify[0].GetSymbolId() != 0 || handler.modify[1].GetSymbolId() != 5 {
		t.Fatalf("modify requests=%v", handler.modify)
	}
	if len(handler.resumes) != 2 || handler.resumes[0].GetSymbolId() != 0 || handler.resumes[1].GetSymbolId() != 5 {
		t.Fatalf("resume requests=%v", handler.resumes)
	}
}

func TestTriggerModifyRejectsUnknownSymbol(t *testing.T) {
	service := newTriggerControlService(t, &triggerControlHandler{})
	unknown := "NOPE-USDT"
	bps := int32(50)
	if _, err := service.Modify(context.Background(), nil, codecs.FormatID(9), nil, &unknown, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &bps}); err == nil {
		t.Fatal("an unknown symbol must fail closed")
	}
}
