package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

func trailingMaximumService(t *testing.T) (*OrdersService, *atomic.Int32) {
	t.Helper()
	var sent atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			sent.Add(1)
		}
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewOrdersService(transport.NewFactory(cfg, creds, nil), auditCatalog(t), nil, nil, nil, nil, nil), &sent
}

func trailingRisk(limit string) *models.AttachedRiskInput {
	bps := int32(100)
	price := models.PriceFromDecimal(limit)
	return &models.AttachedRiskInput{TrailingStop: &models.TrailingStopInput{DistanceBps: &bps, LimitPrice: &price}}
}

// A zero or negative buy-back maximum on modify or batch replace names the
// field, the contract's rule and what to do instead, before anything is sent.
func TestModifyAndBatchReplaceExplainABadTrailingMaximum(t *testing.T) {
	calls := map[string]func(*OrdersService, *models.AttachedRiskInput) error{
		"modify": func(s *OrdersService, risk *models.AttachedRiskInput) error {
			_, err := s.ModifyWith(context.Background(), nil, ModifyParams{
				Symbol: "BTC-USDT", Key: models.OrderKeyByID("100"), NewAttachedRisk: risk,
			})
			return err
		},
		"batch replace": func(s *OrdersService, risk *models.AttachedRiskInput) error {
			_, err := s.BatchReplace(context.Background(), nil, []models.BatchReplaceItem{{
				Key: models.OrderKeyByID("100"), NewAttachedRisk: risk,
			}}, "BTC-USDT", nil, nil)
			return err
		},
	}
	for name, call := range calls {
		for _, limit := range []string{"0", "0.000000000"} {
			t.Run(name+"/"+limit, func(t *testing.T) {
				service, sent := trailingMaximumService(t)
				err := call(service, trailingRisk(limit))
				var validation *sdkerrors.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("got %T %v", err, err)
				}
				if validation.Code != "ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT" {
					t.Fatalf("code=%q", validation.Code)
				}
				if len(validation.Violations) != 1 ||
					!strings.HasSuffix(validation.Violations[0].FieldPath, "new_attached_risk.trailing_stop.limit_price_ticks") ||
					validation.Violations[0].RuleID != "buy_required" {
					t.Fatalf("violations=%+v", validation.Violations)
				}
				for _, want := range []string{"must be positive", "maximum price", "parent order sells", "unset", "parent order buys"} {
					if !strings.Contains(validation.Msg, want) {
						t.Fatalf("message %q lacks %q", validation.Msg, want)
					}
				}
				if sent.Load() != 0 {
					t.Fatalf("sent %d requests", sent.Load())
				}
			})
		}
	}
}

// A positive maximum still reaches the server on both paths, which proves the
// refusal above comes from the value and not from the path.
func TestModifyAndBatchReplaceSendAPositiveTrailingMaximumControl(t *testing.T) {
	service, sent := trailingMaximumService(t)
	risk := trailingRisk("185.5")
	if _, err := service.ModifyWith(context.Background(), nil, ModifyParams{
		Symbol: "BTC-USDT", Key: models.OrderKeyByID("100"), NewAttachedRisk: risk,
	}); err == nil {
		t.Fatal("the test server refuses every request")
	}
	if _, err := service.BatchReplace(context.Background(), nil, []models.BatchReplaceItem{{
		Key: models.OrderKeyByID("100"), NewAttachedRisk: risk,
	}}, "BTC-USDT", nil, nil); err == nil {
		t.Fatal("the test server refuses every request")
	}
	if sent.Load() != 2 {
		t.Fatalf("sent %d requests, want 2", sent.Load())
	}
}
