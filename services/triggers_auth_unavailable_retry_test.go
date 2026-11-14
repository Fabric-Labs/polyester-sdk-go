package services

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// authUnavailableFirst answers the first `failures` requests with a REST 503
// AUTH_TEMPORARILY_UNAVAILABLE problem, as the API's authentication layer
// does, and serves the rest with next.
func authUnavailableFirst(failures int64, next http.Handler) (http.Handler, *atomic.Int64) {
	calls := new(atomic.Int64)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= failures {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"type":"https://polyester.dev/problems/auth-temporarily-unavailable","title":"Authentication Temporarily Unavailable","status":503,"detail":"Authentication is temporarily unavailable.","code":"AUTH_TEMPORARILY_UNAVAILABLE"}`))
			return
		}
		next.ServeHTTP(w, r)
	}), calls
}

func newTriggerServiceBehind(t *testing.T, wrap func(http.Handler) http.Handler, handler triggersv1connect.TriggersServiceHandler) *TriggersService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := triggersv1connect.NewTriggersServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(wrap(mux))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
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

// Authentication that was briefly unavailable changed nothing, so the trigger
// reads that retry an unavailable answer retry it over REST as over ConnectRPC.
func TestTriggerReadsRetryARESTAuthUnavailableAnswer(t *testing.T) {
	symbol := "BTC-USDT"
	for name, read := range map[string]func(*TriggersService) error{
		"Get": func(s *TriggersService) error {
			_, err := s.Get(context.Background(), nil, codecs.FormatID(9), nil)
			return err
		},
		"List": func(s *TriggersService) error {
			_, err := s.List(context.Background(), nil, nil, &symbol, []string{"paused"}, 10, nil)
			return err
		},
		"ListEvents": func(s *TriggersService) error {
			_, err := s.ListEvents(context.Background(), nil, codecs.FormatID(9), nil, 17, nil, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			var calls *atomic.Int64
			service := newTriggerServiceBehind(t, func(next http.Handler) http.Handler {
				wrapped, counter := authUnavailableFirst(1, next)
				calls = counter
				return wrapped
			}, &triggerReadHandler{})
			if err := read(service); err != nil {
				t.Fatalf("read did not recover: %T %v", err, err)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("requests = %d, want 2", got)
			}
		})
	}
}

// The ConnectRPC answer, unavailable with the auth detail, is retried alike.
func TestTriggerReadsRetryAConnectAuthUnavailableAnswer(t *testing.T) {
	handler := &authUnavailableReadHandler{failures: 1}
	service := newTriggerServiceBehind(t, func(next http.Handler) http.Handler { return next }, handler)
	if _, err := service.Get(context.Background(), nil, codecs.FormatID(9), nil); err != nil {
		t.Fatalf("read did not recover: %T %v", err, err)
	}
	if got := handler.calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

type authUnavailableReadHandler struct {
	triggerReadHandler
	failures int64
	calls    atomic.Int64
}

func (h *authUnavailableReadHandler) answer() error {
	if h.calls.Add(1) <= h.failures {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authentication temporarily unavailable"))
		if detail, detailErr := connect.NewErrorDetail(&authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_TEMPORARILY_UNAVAILABLE}); detailErr == nil {
			err.AddDetail(detail)
		}
		return err
	}
	return nil
}

func (h *authUnavailableReadHandler) GetTrigger(ctx context.Context, req *connect.Request[triggersv1.GetTriggerRequest]) (*connect.Response[triggersv1.GetTriggerResponse], error) {
	if err := h.answer(); err != nil {
		return nil, err
	}
	return h.triggerReadHandler.GetTrigger(ctx, req)
}

// A trigger write is never sent again: the refusal is returned after one
// request, retryable by the caller with a known outcome.
func TestTriggerWritesSendARESTAuthUnavailableAnswerOnce(t *testing.T) {
	var calls *atomic.Int64
	service := newTriggerServiceBehind(t, func(next http.Handler) http.Handler {
		wrapped, counter := authUnavailableFirst(1, next)
		calls = counter
		return wrapped
	}, &triggerReadHandler{})
	_, err := service.Cancel(context.Background(), nil, codecs.FormatID(9), nil)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != sdkerrors.AuthCodeTemporarilyUnavailable {
		t.Fatalf("err = %T %v", err, err)
	}
	if !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}
