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
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
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
