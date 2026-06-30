package transport

import (
	"context"
	"crypto/ed25519"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
)

func TestAPIKeyInterceptorSetsSignatureHeaders(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := NewAPIKeyInterceptor(&auth.Credentials{KeyID: "ak_test", PrivateKey: private}, "https://api.example.test")
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
