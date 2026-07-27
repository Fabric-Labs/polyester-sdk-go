package useragent

import "net/http"

// RoundTripper sets the Polyester User-Agent when the request does not already
// have one. Wrap the default transport so Connect, realtime token, and any
// other callers sharing the SDK http.Client inherit the identity.
type RoundTripper struct {
	Base http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Header.Get(Header) == "" {
		// Clone before mutating so we do not race callers that reuse *http.Request.
		clone := req.Clone(req.Context())
		clone.Header.Set(Header, String())
		req = clone
	}
	return base.RoundTrip(req)
}

// WrapClient ensures client uses a RoundTripper that injects User-Agent.
// The returned client may share the same Timeout/Jar/CheckRedirect settings.
func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	if _, ok := client.Transport.(RoundTripper); ok {
		return client
	}
	wrapped := *client
	wrapped.Transport = RoundTripper{Base: client.Transport}
	return &wrapped
}
