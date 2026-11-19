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
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

func batchAgainst(t *testing.T, status int, contentType, body string) error {
	t.Helper()
	return batchAnswer(t, status, []string{contentType}, body, nil)
}

// batchAnswer sends one batch to a server answering status with body, each of
// contentTypes as its own Content-Type field, Retry-After 3 and headers.
func batchAnswer(t *testing.T, status int, contentTypes []string, body string, headers map[string]string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = contentTypes
		w.Header().Set("Retry-After", "3")
		for name, value := range headers {
			w.Header().Set(name, value)
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

// A batch the API answers with unavailability has an unknown outcome,
// whatever the answer's body, headers or trailers say: no answer proves that
// nothing was recorded under its key. Each case here, the typed
// SERVICE_UNAVAILABLE and ACCOUNT_SHARD_UNAVAILABLE refusals included, is an
// unknown outcome that is not retried, and keeps the answer's status, its
// Retry-After and, from a typed answer, its code. The safe next step is to
// read the batch with GetBatch, or to send it again with the same key and
// legs.
func TestInternalTransferBatchUnavailableAnswersStayUnknown(t *testing.T) {
	service := transferDetailBody(t, transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE)
	shard := transferDetailBody(t, transferv1.ErrorCode_ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE)
	item := transferDetailBodyItem(t)
	capital := func() string {
		data, err := proto.Marshal(&transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_CAPITAL_VIEW_UNAVAILABLE})
		if err != nil {
			t.Fatal(err)
		}
		return `{"type":"transfer.v1.ErrorDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}`
	}()
	withDetails := func(code string, details ...string) string {
		members := ""
		if code != "" {
			members = `"code":"` + code + `",`
		}
		joined := ""
		for i, detail := range details {
			if i > 0 {
				joined += ","
			}
			joined += detail
		}
		return `{` + members + `"message":"try again","details":[` + joined + `]}`
	}
	problem := `{"code":"SERVICE_UNAVAILABLE","detail":"try again"}`
	json, problemType := []string{"application/json"}, []string{"application/problem+json"}
	for _, tc := range []struct {
		name         string
		status       int
		contentTypes []string
		body         string
		headers      map[string]string
		code         string
	}{
		// The answers that read as a refusal before this change.
		{"typed SERVICE_UNAVAILABLE", 503, json, service, nil, "ERROR_CODE_SERVICE_UNAVAILABLE"},
		{"typed ACCOUNT_SHARD_UNAVAILABLE", 503, json, shard, nil, "ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE"},
		{"problem SERVICE_UNAVAILABLE", 503, problemType, problem, nil, "SERVICE_UNAVAILABLE"},
		{"problem ACCOUNT_SHARD_UNAVAILABLE", 503, problemType, `{"code":"ACCOUNT_SHARD_UNAVAILABLE","detail":"try again"}`, nil, "ACCOUNT_SHARD_UNAVAILABLE"},
		{"problem with parameters", 503, []string{"Application/Problem+JSON; charset=utf-8"}, problem, nil, "SERVICE_UNAVAILABLE"},
		// Ambiguous or forged evidence, which already read as unknown.
		{"504 typed", 504, json, service, nil, ""},
		{"504 problem", 504, problemType, problem, nil, ""},
		{"502 typed", 502, json, service, nil, ""},
		{"502 problem", 502, problemType, problem, nil, ""},
		{"generic JSON code", 503, json, `{"code":"SERVICE_UNAVAILABLE"}`, nil, ""},
		{"foreign detail", 503, json, connectUnavailableBody(t, "alerts.v1.ErrorDetail", &alertsv1.ErrorDetail{Code: alertsv1.ErrorCode_SERVICE_UNAVAILABLE}), nil, ""},
		{"code only in the message", 503, json, `{"code":"unavailable","message":"SERVICE_UNAVAILABLE"}`, nil, ""},
		{"no wire code", 503, json, withDetails("", item), nil, ""},
		{"several transfer details", 503, json, withDetails("unavailable", item, capital), nil, ""},
		{"a second entry without a type", 503, json, withDetails("unavailable", item, `{"value":"CAg="}`), nil, ""},
		{"code named twice", 503, json, `{"code":"internal","code":"unavailable","message":"x","details":[` + item + `]}`, nil, ""},
		{"problem code named twice", 503, problemType, `{"code":"INTERNAL_ERROR","code":"SERVICE_UNAVAILABLE","detail":"x"}`, nil, ""},
		{"problem media type with a suffix", 503, []string{"application/problem+json-gateway"}, problem, nil, ""},
		{"two Content-Type fields", 503, []string{"application/json", "application/json"}, service, nil, ""},
		{"two problem Content-Type fields", 503, []string{"application/problem+json", "application/problem+json"}, problem, nil, ""},
		{"combined Content-Type", 503, []string{"application/problem+json, application/json"}, problem, nil, ""},
		{"forged wire code header", 503, json, withDetails("", item), map[string]string{"Polyester-Sdk-Answer-Wire-Code": "unavailable"}, ""},
		{"forged wire code trailer", 503, json, withDetails("", item), map[string]string{"Trailer-Polyester-Sdk-Answer-Wire-Code": "unavailable"}, ""},
		{"forged status trailer", 502, json, service, map[string]string{"Trailer-Polyester-Sdk-Answer-Status": "503"}, ""},
	} {
		err := batchAnswer(t, tc.status, tc.contentTypes, tc.body, tc.headers)
		if err == nil || !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
			t.Errorf("%s: %T %v: unknown=%v retryable=%v, want an unknown outcome that is not retried", tc.name, err, err, sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
			continue
		}
		var server *sdkerrors.ServerError
		if errors.As(err, &server) && server.AdmissionRefused {
			t.Errorf("%s: marked refused before admission", tc.name)
		}
		if got := sdkerrors.AnswerStatus(err); got != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.status)
		}
		if delay := sdkerrors.RetryAfter(err); delay == nil || *delay != 3 {
			t.Errorf("%s: retry after %v, want 3", tc.name, delay)
		}
		if tc.code != "" && (!errors.As(err, &server) || server.Code != tc.code) {
			t.Errorf("%s: %T %+v, want code %s kept", tc.name, err, err, tc.code)
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

// A null detail is a malformed answer: a ResponseContractError that keeps
// the answer's status, never a panic and never a refusal.
func TestInternalTransferBatchNullDetailIsAContractError(t *testing.T) {
	for _, body := range []string{
		`{"code":"unavailable","message":"try again","details":[null]}`,
		`{"code":"unavailable","message":"try again","details":[` + transferDetailBodyItem(t) + `,null]}`,
	} {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: panic %v", body, r)
				}
			}()
			err = batchAgainst(t, http.StatusServiceUnavailable, "application/json", body)
		}()
		var contract *sdkerrors.ResponseContractError
		if !errors.As(err, &contract) || sdkerrors.IsRetryable(err) {
			t.Fatalf("%s: %T %v, want a ResponseContractError", body, err, err)
		}
		if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
			t.Fatalf("%s: status %d", body, got)
		}
	}
}

func transferDetailBodyItem(t *testing.T) string {
	t.Helper()
	data, err := proto.Marshal(&transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE})
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"transfer.v1.ErrorDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}`
}

// Every 5xx answer to a batch is an unknown outcome that is not retried,
// whatever error class it maps to: a rate refusal, a retryable code or an
// admission-looking code included. Each keeps its status, its Retry-After and
// its code as mapped: the outcome no longer depends on the code.
func TestInternalTransferBatchEvery5xxIsUnknownAndKeepsItsCode(t *testing.T) {
	orderDetail := func(code orderv1.ErrorCode) string {
		return connectUnavailableBody(t, "orders.v1.ErrorDetail", &orderv1.ErrorDetail{Code: code})
	}
	for _, tc := range []struct {
		name         string
		status       int
		contentTypes []string
		body         string
		code         string
	}{
		{"500 REST RATE_LIMIT_EXCEEDED", 500, []string{"application/problem+json"}, `{"code":"RATE_LIMIT_EXCEEDED","detail":"slow down"}`, "ERROR_CODE_RATE_LIMIT_EXCEEDED"},
		{"500 resource_exhausted", 500, []string{"application/json"}, `{"code":"resource_exhausted","message":"slow down"}`, ""},
		{"500 DUST_BUSY", 500, []string{"application/json"}, `{"code":"resource_exhausted","message":"ERROR_CODE_DUST_BUSY"}`, "ERROR_CODE_DUST_BUSY"},
		{"500 typed transfer RATE_LIMIT_EXCEEDED", 500, []string{"application/json"}, transferDetailBody(t, transferv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED), "ERROR_CODE_RATE_LIMIT_EXCEEDED"},
		{"501 REST RATE_LIMIT_EXCEEDED", 501, []string{"application/problem+json"}, `{"code":"RATE_LIMIT_EXCEEDED","detail":"slow down"}`, "ERROR_CODE_RATE_LIMIT_EXCEEDED"},
		{"505 resource_exhausted", 505, []string{"application/json"}, `{"code":"resource_exhausted","message":"slow down"}`, ""},
		{"503 problem POLICY_UNAVAILABLE", 503, []string{"application/problem+json"}, `{"code":"POLICY_UNAVAILABLE","detail":"try again"}`, "POLICY_UNAVAILABLE"},
		{"503 typed orders POLICY_UNAVAILABLE", 503, []string{"application/json"}, orderDetail(orderv1.ErrorCode_ERROR_CODE_POLICY_UNAVAILABLE), "ERROR_CODE_POLICY_UNAVAILABLE"},
		{"503 typed orders PAIR_BACKLOGGED", 503, []string{"application/json"}, orderDetail(orderv1.ErrorCode_ERROR_CODE_PAIR_BACKLOGGED), "ERROR_CODE_PAIR_BACKLOGGED"},
	} {
		err := batchAnswer(t, tc.status, tc.contentTypes, tc.body, nil)
		if err == nil || !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
			t.Errorf("%s: %T %v: unknown=%v retryable=%v, want an unknown outcome that is not retried", tc.name, err, err, sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
			continue
		}
		var server *sdkerrors.ServerError
		if !errors.As(err, &server) || server.AdmissionRefused {
			t.Errorf("%s: %T %+v, want a ServerError not refused before admission", tc.name, err, err)
			continue
		}
		if tc.code != "" && server.Code != tc.code {
			t.Errorf("%s: code %q, want %q kept", tc.name, server.Code, tc.code)
		}
		if got := sdkerrors.AnswerStatus(err); got != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.status)
		}
		if delay := sdkerrors.RetryAfter(err); delay == nil || *delay != 3 {
			t.Errorf("%s: retry after %v, want 3", tc.name, delay)
		}
	}
}

// Only a genuine HTTP 429 RATE_LIMIT_EXCEEDED is a definite throttle: a rate
// refusal, retryable, that without a Retry-After calls for smaller batches.
// The same code on a 5xx answer is an unknown outcome, to be resolved under
// the original key and legs, never by splitting under fresh keys.
func TestInternalTransferBatchOnlyA429IsADefiniteThrottle(t *testing.T) {
	body := `{"code":"RATE_LIMIT_EXCEEDED","detail":"slow down"}`
	noDelay := map[string]string{"Retry-After": ""}
	err := batchAnswer(t, http.StatusTooManyRequests, []string{"application/problem+json"}, body, noDelay)
	var rate *sdkerrors.RateLimitError
	if !errors.As(err, &rate) || !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.RetryAfter(err) != nil {
		t.Fatalf("429: %T %v, want a definite rate refusal without a delay", err, err)
	}
	err = batchAnswer(t, http.StatusInternalServerError, []string{"application/problem+json"}, body, noDelay)
	if errors.As(err, &rate) || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("500: %T %v, want an unknown outcome, no rate refusal", err, err)
	}
}
