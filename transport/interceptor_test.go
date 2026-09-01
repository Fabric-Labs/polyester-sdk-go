package transport

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"google.golang.org/protobuf/proto"
)

func TestAPIKeyInterceptorSetsSignatureHeaders(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := NewAPIKeyInterceptor(&auth.Credentials{KeyID: "ak_test", PrivateKey: private}, "https://api.example.test", connectx.WireBinary)
	next := func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Header().Get("X-API-SIGNATURE") == "" {
			t.Fatal("expected signature header")
		}
		if req.Header().Get("X-API-KEY-ID") != "ak_test" {
			t.Fatalf("key id %q", req.Header().Get("X-API-KEY-ID"))
		}
		return connect.NewResponse(&authv1.MeResponse{}), nil
	}
	_, err = interceptor.WrapUnary(next)(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodeWireBodyJSONDiffersFromBinary(t *testing.T) {
	msg := &authv1.GetNonceRequest{SmartAccountAddress: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	bin, err := encodeWireBody(msg, connectx.WireBinary)
	if err != nil {
		t.Fatal(err)
	}
	js, err := encodeWireBody(msg, connectx.WireJSON)
	if err != nil {
		t.Fatal(err)
	}
	if string(bin) == string(js) {
		t.Fatal("expected JSON and binary wire bodies to differ for signing")
	}
	if len(js) == 0 || js[0] != '{' {
		t.Fatalf("expected ProtoJSON object, got %q", js)
	}
}

func TestAuthenticatedUnarySignsTransmittedCodecBytes(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := &authv1.GetNonceRequest{SmartAccountAddress: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	cases := []struct {
		name        string
		wire        connectx.WireFormat
		contentType string
	}{
		{name: "binary", wire: connectx.WireBinary, contentType: "application/proto"},
		{name: "json", wire: connectx.WireJSON, contentType: "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				gotBody        []byte
				gotContentType string
				gotKeyID       string
				gotTimestamp   string
				gotSignature   string
				gotPath        string
				gotMethod      string
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotContentType = r.Header.Get("Content-Type")
				gotKeyID = r.Header.Get("X-API-KEY-ID")
				gotTimestamp = r.Header.Get("X-API-TIMESTAMP")
				gotSignature = r.Header.Get("X-API-SIGNATURE")
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
					http.Error(w, "read body", http.StatusInternalServerError)
					return
				}
				gotBody = body
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Connect-Protocol-Version", connectx.ProtocolVersion)
				if tc.wire == connectx.WireJSON {
					_, _ = w.Write([]byte("{}"))
					return
				}
				out, err := proto.Marshal(&authv1.GetNonceResponse{})
				if err != nil {
					t.Errorf("marshal response: %v", err)
					http.Error(w, "marshal", http.StatusInternalServerError)
					return
				}
				_, _ = w.Write(out)
			}))
			t.Cleanup(srv.Close)

			factory := NewFactory(Config{
				APIURL:     srv.URL,
				Timeout:    2 * time.Second,
				WireFormat: tc.wire,
			}, &auth.Credentials{KeyID: "ak_test", PrivateKey: private}, srv.Client())
			client := authv1connect.NewAuthServiceClient(factory.HTTP, factory.Config.APIURL, factory.ConnectOptions(true)...)
			if _, err := client.GetNonce(context.Background(), connect.NewRequest(req)); err != nil {
				t.Fatal(err)
			}

			mediaType := strings.Split(gotContentType, ";")[0]
			if mediaType != tc.contentType {
				t.Fatalf("content type: got %q want %q", gotContentType, tc.contentType)
			}
			if strings.Contains(mediaType, "connect+") {
				t.Fatalf("unary request used streaming media type %q", gotContentType)
			}
			if gotKeyID != "ak_test" {
				t.Fatalf("key id %q", gotKeyID)
			}
			if gotPath != authv1connect.AuthServiceGetNonceProcedure {
				t.Fatalf("path %q", gotPath)
			}

			wantBody, err := encodeWireBody(req, tc.wire)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotBody) != string(wantBody) {
				t.Fatalf("transmitted body does not match active codec: got %q want %q", gotBody, wantBody)
			}

			signURL := auth.RequestURL(srv.URL, gotPath)
			canonical, err := auth.CanonicalSigningString(gotTimestamp, gotMethod, signURL, gotBody)
			if err != nil {
				t.Fatal(err)
			}
			sig, err := hex.DecodeString(gotSignature)
			if err != nil {
				t.Fatal(err)
			}
			if !ed25519.Verify(public, []byte(canonical), sig) {
				t.Fatal("signature does not cover the transmitted request body")
			}
		})
	}
}

func TestSigningFailureMapsToAuthError(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("signing failure must happen before network I/O")
	}))
	t.Cleanup(srv.Close)

	interceptor := NewAPIKeyInterceptor(
		&auth.Credentials{KeyID: "ak_test", PrivateKey: private},
		"://malformed-signing-base",
		connectx.WireBinary,
	)
	client := authv1connect.NewAuthServiceClient(
		srv.Client(),
		srv.URL,
		connect.WithInterceptors(interceptor),
	)
	_, callErr := client.GetNonce(context.Background(), connect.NewRequest(&authv1.GetNonceRequest{
		SmartAccountAddress: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}))
	if callErr == nil {
		t.Fatal("expected signing failure")
	}
	var originalAuthErr *sdkerrors.AuthError
	if !errors.As(callErr, &originalAuthErr) {
		t.Fatalf("Connect call did not retain signing cause: %T (%v)", callErr, callErr)
	}
	mapped := MapError(callErr)
	var authErr *sdkerrors.AuthError
	if !errors.As(mapped, &authErr) {
		t.Fatalf("signing failure mapped to %T (%v), want *errors.AuthError", mapped, mapped)
	}
	if authErr != originalAuthErr {
		t.Fatal("MapError replaced the original signing failure instead of preserving its cause")
	}
	var apiErr *sdkerrors.APIError
	if errors.As(mapped, &apiErr) {
		t.Fatalf("signing failure must not map to APIError: %+v", apiErr)
	}
}
