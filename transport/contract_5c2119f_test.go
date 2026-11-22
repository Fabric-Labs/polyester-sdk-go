package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
)

// rawMeAgainst returns the error a Connect call sees before mapping, whose code
// the read retry selects on.
func rawMeAgainst(t *testing.T, status int, contentType, body string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cfg := DefaultConfig()
	cfg.APIURL = srv.URL
	factory := NewFactory(cfg, nil, nil)
	client := authv1connect.NewAuthServiceClient(factory.HTTP, cfg.APIURL, factory.ConnectOptions(false)...)
	_, err := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
	if err == nil {
		t.Fatal("expected an error")
	}
	return err
}

// A REST temporary failure answers 503 with its problem code, where it
// answered 502 or 504: a dependency unavailable or too slow, or a deadline.
// It reads as ConnectRPC unavailable does: a *errors.ServerError keeping the
// code and status, which reads retry and whose write outcome is unknown.
func TestREST503TemporaryFailureReadsAsConnectUnavailable(t *testing.T) {
	for _, code := range []string{"SERVICE_UNAVAILABLE", "UPSTREAM_ERROR", "WALLET_RESOLUTION_UNAVAILABLE", "DEADLINE_EXCEEDED"} {
		body := fmt.Sprintf(`{"type":"https://polyester.dev/problems/service-unavailable","title":"Service Unavailable","status":503,"detail":"request_deadline_exceeded","code":%q}`, code)
		if raw := rawMeAgainst(t, http.StatusServiceUnavailable, "application/problem+json", body); connect.CodeOf(raw) != connect.CodeUnavailable {
			t.Fatalf("%s: Connect code %v, want unavailable so reads retry it", code, connect.CodeOf(raw))
		}
		err := callMeAgainst(t, http.StatusServiceUnavailable, "application/problem+json", body)
		var server *sdkerrors.ServerError
		if !errors.As(err, &server) || server.Code != code || server.Status != http.StatusServiceUnavailable {
			t.Fatalf("%s: %T %+v", code, err, err)
		}
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%s: retryable=%v unknown=%v, want the outcome of a write unknown", code, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
	}
	// A lowercase Connect code in the body is Connect's: deadline_exceeded
	// reads as ConnectRPC deadline_exceeded.
	err := callMeAgainst(t, http.StatusServiceUnavailable, "application/json", `{"code":"deadline_exceeded","message":"deadline"}`)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("deadline_exceeded: %T %+v", err, err)
	}
}

// A 504 stays an unknown outcome whatever its body, problem or the edge's
// plain text, and is never reported retryable; a plain-text 502 from the
// edge too, since the request may have reached the API.
func TestREST504AndEdgePagesStayUnknownOutcomes(t *testing.T) {
	for _, tc := range []struct {
		status            int
		contentType, body string
	}{
		{http.StatusGatewayTimeout, "application/problem+json", `{"code":"SERVICE_UNAVAILABLE","status":504,"detail":"deadline"}`},
		{http.StatusGatewayTimeout, "text/plain", "error code: 504"},
		{http.StatusBadGateway, "text/plain", "error code: 502"},
	} {
		err := callMeAgainst(t, tc.status, tc.contentType, tc.body)
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%d %q: %T retryable=%v unknown=%v", tc.status, tc.body, err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
		if raw := rawMeAgainst(t, tc.status, tc.contentType, tc.body); connect.CodeOf(raw) != connect.CodeUnavailable {
			t.Fatalf("%d %q: Connect code %v, want unavailable so reads retry it", tc.status, tc.body, connect.CodeOf(raw))
		}
	}
}

// A REST 504 is an unknown outcome whatever retryable code its body names: a
// rate-limit, admission or authentication refusal at 504 does not prove that
// nothing happened. Safe reads still retry it (Connect unavailable). At 429
// the same codes stay retryable refusals with a known outcome.
func TestREST504WithARetryableCodeStaysUnknown(t *testing.T) {
	for _, tc := range []struct{ contentType, body string }{
		{"application/problem+json", `{"code":"RATE_LIMIT_EXCEEDED","status":504,"rateLimit":{"reason":"QUOTA_EXCEEDED","retryAfterMs":"1500"}}`},
		{"application/problem+json", `{"code":"RATE_LIMIT_AUTHORITY_UNAVAILABLE","status":504}`},
		{"application/problem+json", `{"code":"OVERLOADED","status":504}`},
		{"application/problem+json", `{"code":"SYSTEM_RATE_LIMITED","status":504}`},
		{"application/problem+json", `{"code":"REPLAY_CAPACITY_EXHAUSTED","status":504}`},
		{"application/problem+json", `{"code":"AUTH_REPLAY_CAPACITY_EXHAUSTED","status":504}`},
		{"application/problem+json", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":504}`},
		{"application/problem+json", `{"code":"TIMESTAMP_SKEW","status":504}`},
		{"application/problem+json", `{"code":"ORDER_SEQUENCE_EXHAUSTED","status":504}`},
		{"application/json", `{"code":"resource_exhausted","message":"rate limit exceeded, retry later"}`},
	} {
		err := callMeAgainst(t, http.StatusGatewayTimeout, tc.contentType, tc.body, map[string]string{"Retry-After": "2"})
		var server *sdkerrors.ServerError
		if !errors.As(err, &server) || server.Status != http.StatusGatewayTimeout {
			t.Fatalf("%s: %T %+v, want a 504 *errors.ServerError", tc.body, err, err)
		}
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%s: retryable=%v unknown=%v, want an unknown outcome", tc.body, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
		if raw := rawMeAgainst(t, http.StatusGatewayTimeout, tc.contentType, tc.body); connect.CodeOf(raw) != connect.CodeUnavailable {
			t.Fatalf("%s: Connect code %v, want unavailable so safe reads retry", tc.body, connect.CodeOf(raw))
		}
	}
	// Controls: the same refusals at 429 are retryable with a known outcome.
	for _, body := range []string{
		`{"code":"RATE_LIMIT_EXCEEDED","status":429,"rateLimit":{"reason":"QUOTA_EXCEEDED","retryAfterMs":"1500"}}`,
		`{"code":"AUTH_REPLAY_CAPACITY_EXHAUSTED","status":429}`,
	} {
		err := callMeAgainst(t, http.StatusTooManyRequests, "application/problem+json", body)
		var rate *sdkerrors.RateLimitError
		if !errors.As(err, &rate) || !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("429 %s: %T retryable=%v unknown=%v", body, err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
	}
	// An explicit outcome-unknown code at 504 keeps its code.
	err := callMeAgainst(t, http.StatusGatewayTimeout, "application/problem+json", `{"code":"ORDER_CREATE_OUTCOME_UNKNOWN","detail":"read back first"}`)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != "ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN" || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("outcome unknown at 504: %T %+v", err, err)
	}
}

// A 503 keeps its code and the delay its Retry-After asks for, on a REST
// problem and on a ConnectRPC unavailable answer alike.
func TestREST503KeepsItsCodeAndRetryAfter(t *testing.T) {
	err := callMeAgainst(t, http.StatusServiceUnavailable, "application/problem+json",
		`{"code":"UPSTREAM_ERROR","status":503,"detail":"upstream_unavailable"}`, map[string]string{"Retry-After": "30"})
	var server *sdkerrors.ServerError
	if !errors.As(err, &server) || server.Code != "UPSTREAM_ERROR" || server.Status != http.StatusServiceUnavailable {
		t.Fatalf("%T %+v", err, err)
	}
	if server.RetryAfter == nil || *server.RetryAfter != 30 {
		t.Fatalf("RetryAfter = %v, want 30", server.RetryAfter)
	}
	err = callMeAgainst(t, http.StatusServiceUnavailable, "application/json",
		`{"code":"unavailable","message":"temporarily unavailable"}`, map[string]string{"Retry-After": "7"})
	if !errors.As(err, &server) || server.RetryAfter == nil || *server.RetryAfter != 7 {
		t.Fatalf("Connect unavailable: %T %+v", err, err)
	}
	err = callMeAgainst(t, http.StatusServiceUnavailable, "application/problem+json", `{"code":"SERVICE_UNAVAILABLE","status":503}`)
	if !errors.As(err, &server) || server.RetryAfter != nil {
		t.Fatalf("no Retry-After: %T %+v", err, err)
	}
}
