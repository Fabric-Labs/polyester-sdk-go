package polyester

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1/systemv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

type fakeSystem struct {
	systemv1connect.UnimplementedSystemServiceHandler

	mu     sync.Mutex
	calls  int
	signed bool
}

func (f *fakeSystem) GetVersion(_ context.Context, req *connect.Request[systemv1.GetVersionRequest]) (*connect.Response[systemv1.GetVersionResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	h := req.Header()
	if h.Get("Authorization") != "" || h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
		f.signed = true
	}
	return connect.NewResponse(&systemv1.GetVersionResponse{
		Version: "0.10.0-12-g4e21a2f09670",
		Commit:  "4e21a2f0967068fc944f5229eead01985df301b9",
		// An environment this SDK does not know yet stays a plain string.
		Environment: "staging-eu",
	}), nil
}

func TestGetVersionReturnsTheServedBuildWithoutSigning(t *testing.T) {
	fake := &fakeSystem{}
	mux := http.NewServeMux()
	path, handler := systemv1connect.NewSystemServiceHandler(fake)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	// With credentials configured, the version read is still sent unsigned.
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_version", APIPrivateKey: keypair.SecretKeyHex})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	got, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := models.ServerVersion{
		Version:     "0.10.0-12-g4e21a2f09670",
		Commit:      "4e21a2f0967068fc944f5229eead01985df301b9",
		Environment: "staging-eu",
	}
	if got != want {
		t.Fatalf("GetVersion = %+v, want %+v", got, want)
	}
	if got.Release() != "0.10.0" {
		t.Fatalf("Release() = %q, want 0.10.0", got.Release())
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.calls != 1 || fake.signed {
		t.Fatalf("calls=%d signed=%v; want one unsigned call", fake.calls, fake.signed)
	}
}

func TestServerVersionReleaseOfATaggedBuildIsTheWholeVersion(t *testing.T) {
	if got := (models.ServerVersion{Version: "0.92.9"}).Release(); got != "0.92.9" {
		t.Fatalf("Release() = %q", got)
	}
}
