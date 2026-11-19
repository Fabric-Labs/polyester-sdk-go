package services

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

func batchAgainst(t *testing.T, status int, contentType, body string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
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
	}, "batch-unavailable-1")
	return err
}

func transferDetailBody(t *testing.T, code transferv1.ErrorCode) string {
	t.Helper()
	data, err := proto.Marshal(&transferv1.ErrorDetail{Code: code})
	if err != nil {
		t.Fatal(err)
	}
	return `{"code":"unavailable","message":"try again","details":[{"type":"transfer.v1.ErrorDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}]}`
}

// A batch whose checks cannot be decided now is refused before admission:
// nothing is recorded under its key, so it is retryable with the same key and
// legs, and keeps the answer's status and Retry-After. Over ConnectRPC and as
// a REST problem alike.
func TestInternalTransferBatchUnavailableIsRetryableUnderTheSameKey(t *testing.T) {
	for _, code := range []transferv1.ErrorCode{transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE, transferv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE} {
		for wire, err := range map[string]error{
			"connect": batchAgainst(t, http.StatusServiceUnavailable, "application/json", transferDetailBody(t, code)),
			"rest":    batchAgainst(t, http.StatusServiceUnavailable, "application/problem+json", `{"code":"`+code.String()[len("ERROR_CODE_"):]+`","detail":"try again"}`),
		} {
			var server *sdkerrors.ServerError
			if !errors.As(err, &server) {
				t.Fatalf("%s %s: err = %T %v, want a ServerError", wire, code, err, err)
			}
			if !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
				t.Fatalf("%s %s: retryable=%v unknown=%v, want retryable with a known outcome", wire, code, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
			}
			if status := sdkerrors.AnswerStatus(err); status != http.StatusServiceUnavailable {
				t.Fatalf("%s %s: status %d", wire, code, status)
			}
			if delay := sdkerrors.RetryAfter(err); delay == nil || *delay != 3 {
				t.Fatalf("%s %s: retry after %v", wire, code, delay)
			}
		}
	}
	// Any other unavailable refusal of the batch keeps its unknown outcome,
	// and so does one that only names the code in its message.
	for name, body := range map[string]string{
		"CAPITAL_VIEW_UNAVAILABLE": transferDetailBody(t, transferv1.ErrorCode_ERROR_CODE_CAPITAL_VIEW_UNAVAILABLE),
		"code as the message":      `{"code":"unavailable","message":"SERVICE_UNAVAILABLE"}`,
	} {
		if err := batchAgainst(t, http.StatusServiceUnavailable, "application/json", body); !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%s: %T %v, want an unknown outcome", name, err, err)
		}
	}
}
