package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// maxErrorBodyBytes bounds how much of an error body is inspected for a
// TIMESTAMP_SKEW code.
const maxErrorBodyBytes = 64 * 1024

// timestampSkewRoundTripper reports an API-key TIMESTAMP_SKEW rejection as
// *errors.TimestampSkewError. The gateway answers it with a JSON (or problem
// JSON) body {"code":"TIMESTAMP_SKEW"} that is not a Connect error, so
// Connect would discard the code and surface a bare HTTP 401.
type timestampSkewRoundTripper struct {
	base http.RoundTripper
}

func (t timestampSkewRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp.StatusCode < 400 || !isJSONContentType(resp.Header.Get("Content-Type")) {
		return resp, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	// Hand the caller the whole body: the inspected prefix, then the rest.
	resp.Body = prefixedBody{Reader: io.MultiReader(bytes.NewReader(body), resp.Body), Closer: resp.Body}
	if readErr != nil {
		return resp, nil
	}
	// Connect error codes are lowercase names, so this never matches one.
	var detail struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &detail) == nil && detail.Code == wire.TimestampSkewCode {
		_ = resp.Body.Close()
		return nil, &sdkerrors.TimestampSkewError{
			Msg:    "API key timestamp is outside the allowed skew window",
			Status: resp.StatusCode,
		}
	}
	return resp, nil
}

type prefixedBody struct {
	io.Reader
	io.Closer
}

func isJSONContentType(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "application/json") || strings.HasPrefix(value, "application/problem+json")
}

func wrapTimestampSkew(client *http.Client) *http.Client {
	if _, ok := client.Transport.(timestampSkewRoundTripper); ok {
		return client
	}
	wrapped := *client
	wrapped.Transport = timestampSkewRoundTripper{base: client.Transport}
	return &wrapped
}
