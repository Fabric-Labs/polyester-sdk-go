// Package redact removes credentials from values the SDK prints or returns in
// errors. It is the one place the SDK turns a URL into text.
package redact

import (
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
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

// SecretPlaceholder replaces a client-held secret in text.
const SecretPlaceholder = "[REDACTED]"

// minSecretLength is the shortest client-held secret Text replaces: a shorter
// one is too common to replace in text.
const minSecretLength = 4

var (
	secretsMu    sync.Mutex
	knownSecrets = map[string]bool{}
)

// RegisterSecret remembers a client-held secret, such as an API private key,
// so Text removes it wherever it appears.
func RegisterSecret(secret string) {
	if len(secret) < minSecretLength {
		return
	}
	secretsMu.Lock()
	knownSecrets[secret] = true
	secretsMu.Unlock()
}

// RegisterURLSecrets remembers the userinfo of a URL the client holds, its
// API, WebSocket or proxy URL, and its password, raw and decoded.
func RegisterURLSecrets(raw string) {
	for _, part := range userinfoParts(raw) {
		RegisterSecret(part)
		if decoded, err := url.PathUnescape(part); err == nil {
			RegisterSecret(decoded)
		}
	}
}

func userinfoParts(raw string) []string {
	_, rest, found := strings.Cut(raw, "://")
	if !found {
		return nil
	}
	authority, _, _ := strings.Cut(rest, "/")
	at := strings.LastIndex(authority, "@")
	if at <= 0 {
		return nil
	}
	// The exact userinfo and the password: a user name alone is not secret
	// and too common to replace in text.
	userinfo := authority[:at]
	_, password, _ := strings.Cut(userinfo, ":")
	return []string{userinfo, password}
}

// secretsInEffect returns the registered secrets and the userinfo of the
// proxies the environment sets, longest first.
func secretsInEffect() []string {
	secretsMu.Lock()
	secrets := make([]string, 0, len(knownSecrets))
	for secret := range knownSecrets {
		secrets = append(secrets, secret)
	}
	secretsMu.Unlock()
	for _, name := range proxyEnv {
		for _, part := range userinfoParts(strings.TrimSpace(os.Getenv(name))) {
			// Raw and decoded, as RegisterURLSecrets does.
			forms := []string{part}
			if decoded, err := url.PathUnescape(part); err == nil {
				forms = append(forms, decoded)
			}
			for _, form := range forms {
				if len(form) >= minSecretLength {
					secrets = append(secrets, form)
				}
			}
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func isSchemeByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '+' || b == '.' || b == '-'
}

// ProxyPlaceholder replaces a proxy URL from the environment in text.
const ProxyPlaceholder = "[proxy]"

// Text returns text with every URL in it reduced as URL reduces one, and every proxy
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
	// URLs first, so a registered secret inside one can't make it unparsable.
	text = stripURLSecrets(text)
	for _, secret := range secretsInEffect() {
		text = strings.ReplaceAll(text, secret, SecretPlaceholder)
	}
	return text
}

// stripURLSecrets drops, after each "://" and up to the next whitespace,
// everything up to the last "@" (the userinfo, whatever it holds), and
// everything from the next "?" or "#" on (the query and fragment). A URL that
// URL refuses is replaced by InvalidURL whole. It does not
// parse the URL, so a userinfo or query holding parentheses or quotes is
// dropped whole.
func stripURLSecrets(text string) string {
	var out strings.Builder
	position := 0
	for {
		separator := strings.Index(text[position:], "://")
		if separator < 0 {
			break
		}
		start := position + separator + 3
		end := start
		for end < len(text) && !unicode.IsSpace(rune(text[end])) {
			end++
		}
		token := text[start:end]
		scheme := position + separator
		for scheme > position && isSchemeByte(text[scheme-1]) {
			scheme--
		}
		if URL(text[scheme:end]) == InvalidURL {
			// What the URL parser refuses stays refused: dropped whole.
			out.WriteString(text[position:scheme])
			out.WriteString(InvalidURL)
			position = end
			continue
		}
		// Fail closed: the last "@" anywhere ends a userinfo, even one that
		// holds "?" or "#".
		if at := strings.LastIndex(token, "@"); at >= 0 {
			token = token[at+1:]
		}
		cut := len(token)
		if i := strings.IndexAny(token, "?#"); i >= 0 {
			cut = i
		}
		out.WriteString(text[position:start])
		out.WriteString(token[:cut])
		position = end
	}
	out.WriteString(text[position:])
	return out.String()
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
