package auth

import (
	"crypto/ed25519"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func signedRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := NewCredentials("ak_redirect_test", private)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := SignRequest(creds, http.MethodGet, rawURL, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

// counter answers 200 and counts its requests.
func counter(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func redirector(t *testing.T, status int, location func(*http.Request) string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", location(r))
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// A same-origin redirect of a signed request is refused too: the signature
// covers the path, so it would not verify at another path, and the refusal
// says the redirect stayed within the origin.
func TestRefuseSignedRedirectsRefusesSameOrigin(t *testing.T) {
	var srv *httptest.Server
	var moved atomic.Int32
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			moved.Add(1)
			return
		}
		http.Redirect(w, r, "/moved", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	client := RefuseSignedRedirects(nil)
	_, err := client.Do(signedRequest(t, srv.URL+"/v1/rt/token"))
	var refused *errors.RedirectRefusedError
	if !stderrors.As(err, &refused) {
		t.Fatalf("err = %v, want *errors.RedirectRefusedError", err)
	}
	if !refused.SameOrigin || refused.StatusCode != http.StatusTemporaryRedirect || refused.Origin != srv.URL {
		t.Fatalf("refusal = %+v", *refused)
	}
	if moved.Load() != 0 {
		t.Fatal("the redirected path received the signed request")
	}
}

// The refusal names only the target's origin: never its userinfo, path or
// query.
func TestRefuseSignedRedirectsNamesOnlyTheOrigin(t *testing.T) {
	destination, hits := counter(t)
	host := strings.TrimPrefix(destination.URL, "http://")
	origin, _ := redirector(t, http.StatusPermanentRedirect, func(*http.Request) string {
		return "http://fake-user:fake-password@" + host + "/private/path?q=fake-query"
	})
	_, err := RefuseSignedRedirects(nil).Do(signedRequest(t, origin.URL+"/v1/rt/token"))
	var refused *errors.RedirectRefusedError
	if !stderrors.As(err, &refused) {
		t.Fatalf("err = %v, want *errors.RedirectRefusedError", err)
	}
	if refused.Origin != destination.URL {
		t.Fatalf("Origin = %q, want %q", refused.Origin, destination.URL)
	}
	for _, secret := range []string{"fake-user", "fake-password", "/private/path", "fake-query"} {
		if strings.Contains(refused.Error(), secret) {
			t.Fatalf("refusal %q quotes %q", refused.Error(), secret)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("the destination received the signed request")
	}
}

// Unsigned requests keep today's behaviour: the client's own CheckRedirect,
// or net/http's default of following up to 10 redirects.
func TestRefuseSignedRedirectsKeepsUnsignedPolicy(t *testing.T) {
	destination, hits := counter(t)
	origin, _ := redirector(t, http.StatusTemporaryRedirect, func(r *http.Request) string {
		return destination.URL + r.URL.Path
	})

	resp, err := RefuseSignedRedirects(nil).Get(origin.URL + "/public")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || hits.Load() != 1 {
		t.Fatalf("unsigned redirect not followed: status %d, destination hits %d", resp.StatusCode, hits.Load())
	}

	caller := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err = RefuseSignedRedirects(caller).Get(origin.URL + "/public")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || hits.Load() != 1 {
		t.Fatalf("caller policy not kept: status %d, destination hits %d", resp.StatusCode, hits.Load())
	}
	if caller.CheckRedirect == nil {
		t.Fatal("the caller's client was modified")
	}

	var loop *httptest.Server
	loop = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, loop.URL+"/again", http.StatusFound)
	}))
	t.Cleanup(loop.Close)
	if _, err := RefuseSignedRedirects(nil).Get(loop.URL); err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("redirect loop err = %v, want net/http's limit", err)
	}
}
