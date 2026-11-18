package transport

import (
	"errors"
	"net/http"
)

// connNotEstablished is the text of the error net/http's HTTP/2 client
// returns when it hands a request to a pooled connection that closed before
// it carried any request. net/http does not export it.
const connNotEstablished = "http2: client conn could not be established"

// maxUnsentResends bounds how often one request is sent again after
// connNotEstablished: each such failure removes one dead connection from the
// pool, and only connections dialed and never used can be left there.
const maxUnsentResends = 4

// unsentResendRoundTripper sends a request again when net/http refused it
// before writing any of it.
//
// An HTTP/2 connection that drops before it carries a request, such as a
// spare one dialed for a request another connection served, stays in
// net/http's pool for a few seconds, and the next request given to it fails
// with connNotEstablished. net/http returns that error only before the
// request's stream is opened, so no byte of the request left the client,
// and it removes the connection from the pool. Sending the same request
// again, on another connection, is then safe for any request, a mutation
// included: the server never saw the first one.
type unsentResendRoundTripper struct {
	base http.RoundTripper
}

func (rt unsentResendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	for resends := 0; ; resends++ {
		resp, err := base.RoundTrip(req)
		if err == nil || resends == maxUnsentResends || !isConnNotEstablished(err) || req.Context().Err() != nil {
			return resp, err
		}
		next := req.Clone(req.Context())
		if req.Body != nil && req.Body != http.NoBody {
			if req.GetBody == nil {
				return resp, err
			}
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return resp, err
			}
			next.Body = body
		}
		req = next
	}
}

// isConnNotEstablished reports whether err is, or wraps, connNotEstablished.
func isConnNotEstablished(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		if err.Error() == connNotEstablished {
			return true
		}
	}
	return false
}
