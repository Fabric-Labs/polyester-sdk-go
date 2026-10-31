package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
)

func callMeAgainst(t *testing.T, status int, contentType, body string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	return MapError(err)
}

func TestTimestampSkewProblemMapsToRetryableSkewError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"title":"Unauthorized","status":401,"detail":"timestamp outside window","code":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) || skew.Status != http.StatusUnauthorized {
		t.Fatalf("err=%T %v", err, err)
	}
	if !sdkerrors.IsRetryable(err) {
		t.Fatal("TIMESTAMP_SKEW is retryable")
	}
}

func TestOtherAuthProblemStaysAuthError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"title":"Unauthorized","status":401,"code":"INVALID_SIGNATURE"}`)
	var skew *sdkerrors.TimestampSkewError
	var authErr *sdkerrors.AuthError
	if errors.As(err, &skew) || !errors.As(err, &authErr) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestConnectTimestampSkewMessageMapsToSkewError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/json",
		`{"code":"unauthenticated","message":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestBareHTTP501MapsToNotImplemented(t *testing.T) {
	err := callMeAgainst(t, http.StatusNotImplemented, "text/plain", "not implemented")
	var notImplemented *sdkerrors.NotImplementedError
	if !errors.As(err, &notImplemented) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestGatewayTimestampSkewJSONMapsToSkewError(t *testing.T) {
	// The API gateway's shape, observed on devnet: a plain JSON body whose
	// code is not a Connect code.
	err := callMeAgainst(t, http.StatusUnauthorized, "application/json", `{"code":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) || skew.Status != http.StatusUnauthorized || !sdkerrors.IsRetryable(err) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestLargeJSONErrorBodyReachesConnectIntact(t *testing.T) {
	// A Connect error whose body exceeds the inspected prefix must still
	// decode with its full message.
	message := strings.Repeat("x", maxErrorBodyBytes+1024)
	err := callMeAgainst(t, http.StatusBadRequest, "application/json",
		`{"code":"invalid_argument","message":"`+message+`"}`)
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Msg != message {
		t.Fatalf("err=%T len=%d", err, len(err.Error()))
	}
}
