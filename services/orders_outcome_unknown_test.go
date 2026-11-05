package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

func TestOrderCreateNeverResendsUnknownOutcome(t *testing.T) {
	for _, tc := range []struct {
		status            int
		contentType, body string
	}{
		{504, "application/problem+json", `{"code":"ORDER_CREATE_OUTCOME_UNKNOWN"}`},
		{504, "text/html", "<html>Gateway Timeout</html>"},
		{503, "application/json", `{"code":"unavailable","details":[{"type":"orders.v1.ErrorDetail","value":"CK8C"}]}`},
		{504, "application/json", `{"code":"deadline_exceeded","details":[{"type":"orders.v1.ErrorDetail","value":"CK8C"}]}`},
	} {
		t.Run(tc.contentType, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/orders.v1.OrdersService/CreateOrder" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				calls.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
			if err != nil {
				t.Fatal(err)
			}
			cfg := transport.DefaultConfig()
			cfg.APIURL = srv.URL
			service := NewOrdersService(transport.NewFactory(cfg, creds, nil), auditCatalog(t), nil, nil, nil, nil, nil)
			symbol := "BTC-USDT"
			_, err = service.Create(context.Background(), models.CreateOrderRequest{
				Symbol: &symbol, Side: "buy", OrderType: "market", Qty: models.QtyFromDecimal("0.001"),
			}, nil)
			if tc.contentType != "text/html" {
				if _, ok := err.(*sdkerrors.APIError); !ok || sdkerrors.IsRetryable(err) {
					t.Fatalf("class=%T retryable=%v", err, sdkerrors.IsRetryable(err))
				}
			}
			if !sdkerrors.MutationOutcomeUnknown(err) || calls.Load() != 1 {
				t.Fatalf("calls=%d error=%T %v", calls.Load(), err, err)
			}
		})
	}
}
