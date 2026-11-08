package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// fakeRESTURLPassword is an obviously fake password placed in URL userinfo.
const fakeRESTURLPassword = "fake-rest-url-password-marker"

// REST errors must not carry the password from the API URL's userinfo,
// whatever the server answers or when it cannot be reached.
func TestRESTErrorsDoNotLeakURLUserinfo(t *testing.T) {
	bodies := map[string]string{
		"problem": `{"type":"about:blank","code":"AUTH_UNAUTHENTICATED","detail":"refused","status":%d}`,
		"plain":   "refused",
	}
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	check := func(label, apiURL string) {
		t.Helper()
		cfg := transport.DefaultConfig()
		cfg.APIURL = apiURL
		_, err := NewTriggersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil).Get(context.Background(), nil, "9", nil)
		if err == nil {
			t.Fatalf("%s: want an error", label)
		}
		for _, rendered := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
			if strings.Contains(rendered, fakeRESTURLPassword) {
				t.Errorf("%s: error leaked URL userinfo: %s", label, rendered)
			}
		}
	}
	for _, status := range []int{401, 403, 404, 500} {
		for kind, body := range bodies {
			if kind == "problem" {
				body = fmt.Sprintf(body, status)
			}
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "problem" {
					w.Header().Set("Content-Type", "application/problem+json")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			check(fmt.Sprintf("HTTP %d %s", status, kind), "http://fake-user:"+fakeRESTURLPassword+"@"+strings.TrimPrefix(srv.URL, "http://"))
			srv.Close()
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	check("connection refused", "http://fake-user:"+fakeRESTURLPassword+"@"+addr)
}
