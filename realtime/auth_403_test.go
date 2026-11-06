package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func testCreds(t *testing.T) *auth.Credentials {
	t.Helper()
	kp := auth.GenerateEd25519Keypair()
	creds, err := auth.LoadCredentials("test-key", kp.SecretKeyHex, false)
	if err != nil || creds == nil {
		t.Fatalf("creds: %v", err)
	}
	return creds
}

func TestFetchRTTokenMaps403ToAuthError(t *testing.T) {
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"permission_denied","message":"missing address_book permission"}`))
	}))
	defer srv.Close()

	_, err := fetchRTToken(
		context.Background(),
		&http.Client{Timeout: time.Second},
		testCreds(t),
		srv.URL+"/v1/rt/token",
		"realtime connection token",
	)
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want AuthError, got %T: %v", err, err)
	}
	if authErr.Status != http.StatusForbidden {
		t.Fatalf("status=%d want 403", authErr.Status)
	}
	if authErr.Label != "realtime connection token" {
		t.Fatalf("label=%q", authErr.Label)
	}
	if !strings.Contains(authErr.Body, "permission_denied") {
		t.Fatalf("body=%q", authErr.Body)
	}
	if authErr.Code != "permission_denied" {
		t.Fatalf("code=%q", authErr.Code)
	}
	if authErr.Context != "realtime connection token" {
		t.Fatalf("context=%q", authErr.Context)
	}
	if authErr.Endpoint != srv.URL+"/v1/rt/token" {
		t.Fatalf("endpoint=%q", authErr.Endpoint)
	}
	var rtErr *sdkerrors.RealtimeError
	if errors.As(err, &rtErr) {
		t.Fatalf("must not be RealtimeError: %v", err)
	}
}

func TestFetchRTTokenMaps401ToAuthError(t *testing.T) {
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`unauthorized`))
	}))
	defer srv.Close()

	_, err := fetchRTToken(
		context.Background(),
		&http.Client{Timeout: time.Second},
		testCreds(t),
		srv.URL,
		"realtime connection token",
	)
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want AuthError, got %T: %v", err, err)
	}
	if authErr.Status != http.StatusUnauthorized {
		t.Fatalf("status=%d", authErr.Status)
	}
	if authErr.Code != "unauthenticated" {
		t.Fatalf("code=%q", authErr.Code)
	}
}

func TestRealtimeProblemPreservesMetadata(t *testing.T) {
	for _, status := range []int{401, 403} {
		for _, suppliedType := range []string{"", "https://polyester.dev/problems/AUTH_SESSION_KIND_NOT_ALLOWED", "https://polyester.dev/problems/auth-session-kind-not-allowed"} {
			body := map[string]any{"code": "AUTH_SESSION_KIND_NOT_ALLOWED", "detail": "", "status": 403}
			if suppliedType != "" {
				body["type"] = suppliedType
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(status)
				_, _ = w.Write(encoded)
			}))
			_, err = fetchRTToken(context.Background(), srv.Client(), testCreds(t), srv.URL, "token")
			srv.Close()
			var authErr *sdkerrors.AuthError
			if !errors.As(err, &authErr) {
				t.Fatalf("error=%T %v", err, err)
			}
			problem := authErr.Problem
			if problem == nil || problem.Code != "AUTH_SESSION_KIND_NOT_ALLOWED" || problem.Detail != "" || problem.Status != status || problem.Type != "https://polyester.dev/problems/auth-session-kind-not-allowed" {
				t.Fatalf("supplied type %q: problem metadata lost: %+v", suppliedType, problem)
			}
		}
	}
}
