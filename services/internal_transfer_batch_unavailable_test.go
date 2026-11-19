package services

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
	"google.golang.org/protobuf/proto"
)

func batchAgainst(t *testing.T, status int, contentType, body string) error {
	t.Helper()
	return batchAgainstDetail(t, status, contentType, body)
}

func batchAgainstDetail(t *testing.T, status int, contentType, body string, headers ...map[string]string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Retry-After", "3")
		for _, extra := range headers {
			for name, value := range extra {
				w.Header().Set(name, value)
			}
		}
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
	return connectUnavailableBody(t, "transfer.v1.ErrorDetail", &transferv1.ErrorDetail{Code: code})
}

func connectUnavailableBody(t *testing.T, typeName string, detail proto.Message) string {
	t.Helper()
	data, err := proto.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	return `{"code":"unavailable","message":"try again","details":[{"type":"` + typeName + `","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}]}`
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

// Only an actual 503 whose code the batch's own typed detail or a contract
// REST problem carries proves that nothing was recorded. A 504 never does,
// nor a 502, a foreign detail or a body that merely looks like a problem:
// each keeps its unknown outcome and is not retried.
func TestInternalTransferBatchAmbiguousUnavailableKeepsItsUnknownOutcome(t *testing.T) {
	cases := map[string]struct {
		status      int
		contentType string
		body        string
	}{}
	for _, code := range []transferv1.ErrorCode{transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE, transferv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE} {
		rest := code.String()[len("ERROR_CODE_"):]
		problem := `{"code":"` + rest + `","detail":"try again"}`
		cases["504 typed detail "+rest] = struct {
			status      int
			contentType string
			body        string
		}{http.StatusGatewayTimeout, "application/json", transferDetailBody(t, code)}
		cases["504 problem "+rest] = struct {
			status      int
			contentType string
			body        string
		}{http.StatusGatewayTimeout, "application/problem+json", problem}
		cases["502 typed detail "+rest] = struct {
			status      int
			contentType string
			body        string
		}{http.StatusBadGateway, "application/json", transferDetailBody(t, code)}
		cases["502 problem "+rest] = struct {
			status      int
			contentType string
			body        string
		}{http.StatusBadGateway, "application/problem+json", problem}
		cases["503 generic JSON "+rest] = struct {
			status      int
			contentType string
			body        string
		}{http.StatusServiceUnavailable, "application/json", `{"code":"` + rest + `"}`}
	}
	cases["503 foreign detail"] = struct {
		status      int
		contentType string
		body        string
	}{http.StatusServiceUnavailable, "application/json", connectUnavailableBody(t, "alerts.v1.ErrorDetail", &alertsv1.ErrorDetail{Code: alertsv1.ErrorCode_SERVICE_UNAVAILABLE})}
	for name, tc := range cases {
		err := batchAgainst(t, tc.status, tc.contentType, tc.body)
		if err == nil || !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
			t.Errorf("%s: %T %v: unknown=%v retryable=%v, want an unknown outcome that is not retried", name, err, err, sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
		}
	}
}

// A REST answer is a contract problem only when its media type is exactly
// application/problem+json, in any case and with parameters; a media type that
// only starts with it, or a Content-Type that does not parse, proves nothing.
func TestInternalTransferBatchProblemNeedsTheProblemMediaType(t *testing.T) {
	body := `{"code":"SERVICE_UNAVAILABLE","detail":"try again"}`
	for contentType, refused := range map[string]bool{
		"application/problem+json":                true,
		"Application/Problem+JSON; charset=utf-8": true,
		"application/problem+json-gateway":        false,
		"application/problem+json; charset":       false,
	} {
		err := batchAgainst(t, http.StatusServiceUnavailable, contentType, body)
		if refused && (!sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err)) {
			t.Errorf("%q: %T %v, want retryable with a known outcome", contentType, err, err)
		}
		if !refused && (!sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err)) {
			t.Errorf("%q: %T %v, want an unknown outcome that is not retried", contentType, err, err)
		}
		if delay := sdkerrors.RetryAfter(err); refused && (delay == nil || *delay != 3) {
			t.Errorf("%q: retry after %v, want 3", contentType, delay)
		}
	}
}

// A batch answered with HTTP 502 or 503 has an unknown outcome whatever the
// answer maps to, a rate refusal or a retryable code included: only the
// verified refusal above proves that nothing was recorded. The status, the
// Retry-After and the answer's code and message are kept for diagnosis.
func TestInternalTransferBatchGatewayStatusesKeepAnUnknownOutcomeInEveryClass(t *testing.T) {
	bodies := map[string]struct {
		contentType string
		body        string
	}{
		"REST RATE_LIMIT_EXCEEDED":           {"application/problem+json", `{"code":"RATE_LIMIT_EXCEEDED","detail":"slow down"}`},
		"typed transfer RATE_LIMIT_EXCEEDED": {"application/json", transferDetailBody(t, transferv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED)},
		"generic resource_exhausted":         {"application/json", `{"code":"resource_exhausted","message":"slow down"}`},
		"resource_exhausted DUST_BUSY":       {"application/json", `{"code":"resource_exhausted","message":"ERROR_CODE_DUST_BUSY"}`},
	}
	for name, tc := range bodies {
		for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable} {
			err := batchAgainst(t, status, tc.contentType, tc.body)
			if err == nil || !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
				t.Errorf("%d %s: %T %v: unknown=%v retryable=%v, want an unknown outcome that is not retried", status, name, err, err, sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
				continue
			}
			if got := sdkerrors.AnswerStatus(err); got != status {
				t.Errorf("%d %s: status %d", status, name, got)
			}
			if delay := sdkerrors.RetryAfter(err); delay == nil || *delay != 3 {
				t.Errorf("%d %s: retry after %v, want 3", status, name, delay)
			}
			var server *sdkerrors.ServerError
			if name == "typed transfer RATE_LIMIT_EXCEEDED" && (!errors.As(err, &server) || server.Code != "ERROR_CODE_RATE_LIMIT_EXCEEDED") {
				t.Errorf("%d %s: %T %+v, want the code kept for diagnosis", status, name, err, err)
			}
		}
	}
	// A rate refusal answered with 429 is one: retryable, as before.
	err := batchAgainst(t, http.StatusTooManyRequests, "application/json", `{"code":"resource_exhausted","message":"slow down"}`)
	var rate *sdkerrors.RateLimitError
	if !errors.As(err, &rate) || !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("429: %T %v, want a retryable RateLimitError", err, err)
	}
}

// The verified refusal is a ConnectRPC error whose wire code is unavailable
// and that carries exactly one transfer.v1.ErrorDetail. A 503 without that
// wire code (connect-go infers unavailable from the status), or with several
// transfer details, conflicting or not, proves nothing: the batch keeps its
// unknown outcome.
func TestInternalTransferBatchRefusalNeedsTheWireCodeAndOneTransferDetail(t *testing.T) {
	detail := func(code transferv1.ErrorCode) string {
		data, err := proto.Marshal(&transferv1.ErrorDetail{Code: code})
		if err != nil {
			t.Fatal(err)
		}
		return `{"type":"transfer.v1.ErrorDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}`
	}
	service := detail(transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE)
	shard := detail(transferv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE)
	capital := detail(transferv1.ErrorCode_ERROR_CODE_CAPITAL_VIEW_UNAVAILABLE)
	for name, tc := range map[string]struct {
		body    string
		refused bool
	}{
		"unavailable with one detail": {`{"code":"unavailable","message":"try again","details":[` + service + `]}`, true},
		"no wire code":                {`{"message":"try again","details":[` + service + `]}`, false},
		"empty wire code":             {`{"code":"","message":"try again","details":[` + service + `]}`, false},
		"only details":                {`{"details":[` + service + `]}`, false},
		"service then capital view":   {`{"code":"unavailable","message":"try again","details":[` + service + `,` + capital + `]}`, false},
		"capital view then service":   {`{"code":"unavailable","message":"try again","details":[` + capital + `,` + service + `]}`, false},
		"service twice":               {`{"code":"unavailable","message":"try again","details":[` + service + `,` + service + `]}`, false},
		"service then account shard":  {`{"code":"unavailable","message":"try again","details":[` + service + `,` + shard + `]}`, false},
	} {
		err := batchAgainst(t, http.StatusServiceUnavailable, "application/json", tc.body)
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
	// The answer cannot name its own wire code through the SDK's header.
	err := batchAgainstDetail(t, http.StatusServiceUnavailable, "application/json", `{"message":"try again","details":[`+service+`]}`,
		map[string]string{wire.AnswerWireCodeHeader: "unavailable"})
	if !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
		t.Fatalf("spoofed wire code: %T %v, want an unknown outcome that is not retried", err, err)
	}
}
