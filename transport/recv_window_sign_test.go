package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"google.golang.org/protobuf/proto"
)

// A configured receive window reaches the wire as the signed
// X-API-RECV-WINDOW header, and the server can verify the signature over the
// payload with the window line; without one, no header is sent.
func TestFactoryRecvWindowIsSentAndSigned(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		window int
		header string
	}{{"window", 2500, "2500"}, {"none", 0, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			var gotHeader, payload string
			var sig []byte
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				sum := sha256.Sum256(body)
				gotHeader = r.Header.Get("X-API-RECV-WINDOW")
				lines := []string{r.Header.Get("X-API-TIMESTAMP")}
				if values := r.Header.Values("X-API-RECV-WINDOW"); len(values) > 0 {
					lines = append(lines, values...)
				}
				payload = strings.Join(append(lines, r.Method, r.URL.Path, "", hex.EncodeToString(sum[:])), "\n")
				sig, _ = hex.DecodeString(r.Header.Get("X-API-SIGNATURE"))
				out, _ := proto.Marshal(&authv1.MeResponse{})
				w.Header().Set("Content-Type", "application/proto")
				_, _ = w.Write(out)
			}))
			t.Cleanup(srv.Close)
			cfg := DefaultConfig()
			cfg.APIURL = srv.URL
			cfg.WireFormat = connectx.WireBinary
			cfg.RecvWindowMS = tc.window
			factory := NewFactory(cfg, testCredentials(t, "ak_test", private), nil)
			client := authv1connect.NewAuthServiceClient(factory.HTTP, cfg.APIURL, factory.ConnectOptions(true)...)
			if _, err := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{})); err != nil {
				t.Fatal(err)
			}
			if gotHeader != tc.header {
				t.Fatalf("X-API-RECV-WINDOW = %q, want %q", gotHeader, tc.header)
			}
			if !ed25519.Verify(public, []byte(payload), sig) {
				t.Fatalf("signature does not verify over %q", payload)
			}
		})
	}
}
