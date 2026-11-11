// Package redact removes credentials from values the SDK prints or returns in
// errors. It is the one place the SDK turns a URL into text.
package redact

import (
	"errors"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

// InvalidURL replaces a URL that cannot be parsed, so its userinfo, query and
// fragment cannot be told apart from the rest of it.
const InvalidURL = "[REDACTED INVALID URL]"

// errMalformedLocation replaces net/http's "failed to parse Location header"
// error, whose text quotes the server-supplied Location twice.
var errMalformedLocation = errors.New("redirect Location is not a valid URL")

// locationParseFailure starts the text of net/http's error for a redirect
// Location that does not parse.
const locationParseFailure = "failed to parse Location header"

// URL returns raw with only its scheme, host, port and path: without its
// userinfo (user name and password), query or fragment. It returns InvalidURL
// when raw does not parse as a URL, or parses as one whose parts cannot be
// told apart.
func URL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return InvalidURL
	}
	if parsed.Opaque != "" ||
		(parsed.Host == "" && (parsed.Scheme != "" || strings.Contains(raw, "@"))) {
		return InvalidURL
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// URLError returns err with the URL of every *url.Error in it, which the
// standard library quotes whole, passed through URL. A *url.Error that
// reports a redirect Location that does not parse has that cause replaced,
// as its text quotes the Location. The cause of any other *url.Error is kept,
// so errors.Is and errors.As still reach it. Other errors are returned
// unchanged.
func URLError(err error) error {
	urlErr, ok := err.(*url.Error)
	if !ok {
		return err
	}
	cause := URLError(urlErr.Err)
	if cause != nil && strings.HasPrefix(cause.Error(), locationParseFailure) {
		cause = errMalformedLocation
	}
	redacted := URL(urlErr.URL)
	if redacted == urlErr.URL && cause == urlErr.Err {
		return err
	}
	return &url.Error{Op: urlErr.Op, URL: redacted, Err: cause}
}

// proxyEnv names the proxy settings net/http reads from the environment.
var proxyEnv = []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"}

// urlInText matches a URL inside free text: up to whitespace, a quote, an
// angle bracket or a parenthesis.
var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>()]+`)

// ProxyPlaceholder replaces a proxy URL from the environment in text.
const ProxyPlaceholder = "[proxy]"

// Text returns text with every URL in it as URL renders it, and every proxy
// URL the environment configures, with or without credentials, replaced by
// ProxyPlaceholder. Use it for text that quotes an answer or a transport,
// such as an error message.
func Text(text string) string {
	for _, name := range proxyEnv {
		proxy := strings.TrimSpace(os.Getenv(name))
		if proxy == "" {
			continue
		}
		forms := []string{proxy, strings.TrimRight(proxy, "/")}
		if stripped := URL(proxy); stripped != InvalidURL {
			forms = append(forms, stripped, strings.TrimRight(stripped, "/"))
		}
		sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
		for _, form := range forms {
			if form != "" {
				text = strings.ReplaceAll(text, form, ProxyPlaceholder)
			}
		}
	}
	return urlInText.ReplaceAllStringFunc(text, URL)
}

// Message returns text with every rendering of a *url.Error wrapped by err
// replaced by the rendering of URLError of it, then passed through Text. text
// is usually err.Error() or a message an outer error copied from it, such as
// a Connect error's.
func Message(text string, err error) string {
	return Text(urlErrorMessage(text, err))
}

func urlErrorMessage(text string, err error) string {
	for e := err; e != nil; e = errors.Unwrap(e) {
		urlErr, ok := e.(*url.Error)
		if !ok {
			continue
		}
		if redacted := URLError(urlErr); redacted != error(urlErr) {
			text = strings.ReplaceAll(text, urlErr.Error(), redacted.Error())
		}
		// Also when an outer error reworded it around the URL.
		if urlErr.URL != "" {
			text = strings.ReplaceAll(text, urlErr.URL, URL(urlErr.URL))
		}
		break
	}
	return text
}
