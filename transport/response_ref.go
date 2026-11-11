package transport

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// responseRefHeaders name a response at the edge, most specific first.
var responseRefHeaders = []string{"Cf-Ray", "X-Request-Id"}

// ResponseRefs collects the edge reference of each response a request made
// with its context received; see WithResponseRefs.
type ResponseRefs struct {
	mu   sync.Mutex
	refs []string
}

type responseRefsKey struct{}

// WithResponseRefs returns a context whose requests through a Factory's HTTP
// clients record the edge reference (cf-ray header) of each response they
// receive, and the collector they record it in.
func WithResponseRefs(ctx context.Context) (context.Context, *ResponseRefs) {
	refs := &ResponseRefs{}
	return context.WithValue(ctx, responseRefsKey{}, refs), refs
}

// Last returns the reference of the last response recorded, or "".
func (r *ResponseRefs) Last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.refs) == 0 {
		return ""
	}
	return r.refs[len(r.refs)-1]
}

type responseRefRoundTripper struct {
	base http.RoundTripper
}

func (t responseRefRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if resp != nil {
		if refs, ok := req.Context().Value(responseRefsKey{}).(*ResponseRefs); ok {
			for _, name := range responseRefHeaders {
				if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
					refs.mu.Lock()
					refs.refs = append(refs.refs, value)
					refs.mu.Unlock()
					break
				}
			}
		}
	}
	return resp, err
}
