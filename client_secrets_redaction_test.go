package polyester

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
)

// POLY-9228: the userinfo of a proxy the client is configured with, or that
// the environment sets, never reaches an error's text. The proxies are
// synthetic and never dialled.
func TestProxyUserinfoIsRedactedFromErrorText(t *testing.T) {
	configured, _ := url.Parse("http://sdk-configured-proxy-user:configured-proxy-secret@127.0.0.1:1")
	transport := &http.Transport{Proxy: http.ProxyURL(configured)}
	client, err := New(Config{APIURL: "http://127.0.0.1:1", HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	t.Setenv("HTTPS_PROXY", "http://sdk-env-proxy-user:env-proxy-secret@127.0.0.1:1")

	text := redact.Text("proxy sdk-configured-proxy-user:configured-proxy-secret refused; " +
		"env sdk-env-proxy-user:env-proxy-secret and env-proxy-secret refused")
	for _, secret := range []string{"sdk-configured-proxy-user:configured-proxy-secret", "configured-proxy-secret", "sdk-env-proxy-user:env-proxy-secret", "env-proxy-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("%q survives: %s", secret, text)
		}
	}
}
