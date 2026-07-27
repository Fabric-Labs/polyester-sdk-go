package useragent_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
)

func TestStringIsPolyesterIdentity(t *testing.T) {
	ua := useragent.String()
	if !strings.HasPrefix(ua, "polyester-sdk-go") {
		t.Fatalf("user-agent=%q", ua)
	}
	if strings.Contains(ua, "Go-http-client") {
		t.Fatalf("must not use Go default user-agent: %q", ua)
	}
}

func TestRoundTripperSetsUserAgent(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := useragent.WrapClient(&http.Client{})
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if got != useragent.String() {
		t.Fatalf("user-agent=%q want %q", got, useragent.String())
	}
}

func TestIsCloudflareBrowserBan(t *testing.T) {
	body := `<!DOCTYPE html><title>Attention Required! | Cloudflare</title>error code: 1010`
	if !useragent.IsCloudflareBrowserBan(body) {
		t.Fatal("expected cloudflare 1010 detection")
	}
	if useragent.IsCloudflareBrowserBan(`{"code":"permission_denied"}`) {
		t.Fatal("did not expect false positive")
	}
}
