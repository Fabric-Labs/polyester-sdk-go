package transport

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	"google.golang.org/protobuf/proto"
)

func detailJSON(t *testing.T, typeName string, msg proto.Message) string {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`{"type":%q,"value":%q}`, typeName, base64.RawStdEncoding.EncodeToString(data))
}

// The HTTP status decides outcome certainty first: a 504 is an unknown
// outcome before any message, code or typed detail is read. These are the
// review's probes: ConnectRPC bodies at 504 whose message or detail names a
// retryable refusal, and plain server failures.
func TestRound3Probe504ConnectBodiesAreUnknown(t *testing.T) {
	rateDetail := detailJSON(t, "orders.v1.ErrorDetail", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED})
	limitDetail := detailJSON(t, "polyester.ratelimit.v1.RateLimitDetail", &ratelimitv1.RateLimitDetail{RetryAfterMs: proto.Uint64(1500)})
	authDetail := `{"type":"auth.v1.AuthErrorDetail","value":""}`
	for _, body := range []string{
		`{"code":"unavailable","message":"TIMESTAMP_SKEW"}`,
		`{"code":"unavailable","message":"rate limited","details":[` + rateDetail + `]}`,
		`{"code":"unavailable","message":"rate limited","details":[` + limitDetail + `]}`,
		`{"code":"resource_exhausted","message":"rate limited","details":[` + rateDetail + `]}`,
		`{"code":"unavailable","message":"AUTH_TEMPORARILY_UNAVAILABLE","details":[` + authDetail + `]}`,
		`{"code":"unavailable","message":"temporarily unavailable"}`,
		`{"code":"internal","message":"boom"}`,
		`{"code":"unknown","message":"?"}`,
	} {
		err := callMeAgainst(t, http.StatusGatewayTimeout, "application/json", body)
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Errorf("504 %s: %T retryable=%v unknown=%v", body, err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
		if raw := rawMeAgainst(t, http.StatusGatewayTimeout, "application/json", body); connect.CodeOf(raw) != connect.CodeUnavailable {
			t.Errorf("504 %s: Connect code %v, want unavailable so safe reads retry", body, connect.CodeOf(raw))
		}
	}
	// An outcome-unknown code at 504 keeps its code even beside a rate-limit
	// detail, and stays unknown.
	body := `{"code":"resource_exhausted","message":"ORDER_CREATE_OUTCOME_UNKNOWN","details":[` + limitDetail + `]}`
	err := callMeAgainst(t, http.StatusGatewayTimeout, "application/json", body)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != "ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN" || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Errorf("504 outcome unknown beside a rate-limit detail: %T %+v", err, err)
	}
}

// Retry-After is captured once for every answer and carried by whatever SDK
// error it maps to, an authentication refusal included.
func TestRound3ProbeRetryAfterOnEveryAnswer(t *testing.T) {
	for _, tc := range []struct {
		status            int
		contentType, body string
	}{
		{503, "application/problem+json", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`},
		{503, "application/problem+json", `{"code":"ACCOUNT_ROUTING_INVALID","status":503}`},
		{503, "application/problem+json", `{"code":"PAIR_BACKLOGGED","status":503}`},
		{503, "application/json", `{"code":"unavailable","message":"AUTH_TEMPORARILY_UNAVAILABLE"}`},
		{429, "application/problem+json", `{"code":"RATE_LIMIT_EXCEEDED","status":429}`},
		{504, "application/problem+json", `{"code":"ORDER_CREATE_OUTCOME_UNKNOWN","status":504}`},
	} {
		err := callMeAgainst(t, tc.status, tc.contentType, tc.body, map[string]string{"Retry-After": "30"})
		if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
			t.Errorf("%d %s: %T retry after %v, want 30", tc.status, tc.body, err, got)
		}
	}
}

// ConnectRPC answers a server-side deadline_exceeded with HTTP 504. That is
// the request's deadline, read as one: a deadline error with an unknown
// outcome, whatever its message says, and never a refusal.
func TestRound3ConnectDeadlineAt504IsTheDeadline(t *testing.T) {
	for _, body := range []string{
		`{"code":"deadline_exceeded","message":"context deadline exceeded"}`,
		`{"code":"deadline_exceeded","message":"TIMESTAMP_SKEW"}`,
	} {
		err := callMeAgainst(t, http.StatusGatewayTimeout, "application/json", body)
		var transportErr *sdkerrors.TransportError
		if !errors.As(err, &transportErr) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s: %T %+v, want the deadline", body, err, err)
		}
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%s: retryable=%v unknown=%v", body, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
		raw := rawMeAgainst(t, http.StatusGatewayTimeout, "application/json", body)
		if !errors.Is(raw, context.DeadlineExceeded) {
			t.Fatalf("%s: raw %v does not wrap the deadline", body, raw)
		}
	}
}
