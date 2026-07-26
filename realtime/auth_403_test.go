package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	var rtErr *sdkerrors.RealtimeError
	if errors.As(err, &rtErr) {
		t.Fatalf("must not be RealtimeError: %v", err)
	}
}

func TestFetchRTTokenMaps401ToAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}
