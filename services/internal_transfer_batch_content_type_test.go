package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// batchAgainstContentTypes answers a batch with a 503 problem whose response
// carries each of contentTypes as its own Content-Type field.
func batchAgainstContentTypes(t *testing.T, contentTypes []string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = contentTypes
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"SERVICE_UNAVAILABLE","detail":"try again"}`))
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	svc := NewInternalTransfersService(transport.NewFactory(cfg, creds, nil), nil, nil)
	scale, dest := 18, "0x2222222222222222222222222222222222222222"
	_, err = svc.CreateBatch(context.Background(), []models.InternalTransferBatchLeg{
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("0.5"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
	}, "batch-content-type-1")
	return err
}

// A contract REST problem has exactly one Content-Type field naming one media
// type. Two fields, or one field combining several media types, say nothing
// certain about the body, so the batch keeps its unknown outcome.
func TestInternalTransferBatchProblemNeedsOneContentType(t *testing.T) {
	for name, tc := range map[string]struct {
		contentTypes []string
		refused      bool
	}{
		"one field":                  {[]string{"application/problem+json; charset=utf-8"}, true},
		"duplicate fields":           {[]string{"application/problem+json", "application/problem+json"}, false},
		"problem then json fields":   {[]string{"application/problem+json", "application/json"}, false},
		"combined media types":       {[]string{"application/problem+json; charset=utf-8, application/json"}, false},
		"combined without parameter": {[]string{"application/problem+json, application/json"}, false},
	} {
		err := batchAgainstContentTypes(t, tc.contentTypes)
		if tc.refused && (!sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err)) {
			t.Errorf("%s: %T %v, want retryable with a known outcome", name, err, err)
		}
		if !tc.refused && (!sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err)) {
			t.Errorf("%s: %T %v, want an unknown outcome that is not retried", name, err, err)
		}
		if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
			t.Errorf("%s: status %d", name, got)
		}
	}
}
