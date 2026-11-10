package auth

import (
	stderrors "errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// signedHeaders are the headers that carry API-key credentials. A request
// with any of them never follows a redirect.
var signedHeaders = [...]string{"X-API-KEY-ID", "X-API-TIMESTAMP", "X-API-SIGNATURE"}

// maxRedirects is net/http's default limit, kept for unsigned requests when
// the client has no redirect policy of its own.
const maxRedirects = 10

// RefuseSignedRedirects returns a copy of client whose redirect policy refuses
// to follow any redirect of a request signed with an API key, returning
// *errors.RedirectRefusedError instead, so the signature, key ID and timestamp
// never reach the redirect target. Unsigned requests keep client's own
// CheckRedirect, or net/http's default of up to 10 redirects. A nil client is
// treated as &http.Client{}.
//
// The policy lives in the http.Client. An http.RoundTripper that follows
// redirects by itself, below the client, is outside it.
func RefuseSignedRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	guarded := *client
	next := client.CheckRedirect
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		for _, sent := range via {
			if isSigned(sent.Header) {
				return redirectRefused(req, via[len(via)-1])
			}
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= maxRedirects {
			return stderrors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &guarded
}

func isSigned(header http.Header) bool {
	for _, name := range signedHeaders {
		if header.Get(name) != "" {
			return true
		}
	}
	return false
}

// redirectRefused describes the redirect that answered previous: its status
// and the origin next would have gone to.
func redirectRefused(next, previous *http.Request) *errors.RedirectRefusedError {
	status := 0
	if next.Response != nil {
		status = next.Response.StatusCode
	}
	return &errors.RedirectRefusedError{
		StatusCode: status,
		Origin:     strings.ToLower(next.URL.Scheme) + "://" + strings.ToLower(next.URL.Host),
		SameOrigin: comparableOrigin(next.URL) == comparableOrigin(previous.URL),
	}
}

// comparableOrigin is u's scheme, host and port, with the scheme's default
// port spelled out so that equal origins compare equal.
func comparableOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https", "wss":
			port = "443"
		case "http", "ws":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}
