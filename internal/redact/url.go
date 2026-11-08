// Package redact removes credentials from values the SDK prints or returns in
// errors.
package redact

import (
	"net/url"
	"strings"
)

// InvalidURL replaces a URL that cannot be parsed, so its userinfo cannot be
// told apart from the rest of it.
const InvalidURL = "[REDACTED INVALID URL]"

// URL returns raw without its userinfo (user name and password), or InvalidURL
// when raw does not parse as a URL.
func URL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return InvalidURL
	}
	if parsed.Opaque != "" ||
		(parsed.Host == "" && (parsed.Scheme != "" || strings.Contains(raw, "@"))) {
		return InvalidURL
	}
	if parsed.User == nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

// URLError returns a *url.Error with its URL passed through URL. The standard
// library quotes the whole URL, userinfo included, in the text of a parse
// error. Other errors are returned unchanged.
func URLError(err error) error {
	urlErr, ok := err.(*url.Error)
	if !ok {
		return err
	}
	redacted := URL(urlErr.URL)
	if redacted == urlErr.URL {
		return err
	}
	return &url.Error{Op: urlErr.Op, URL: redacted, Err: urlErr.Err}
}
