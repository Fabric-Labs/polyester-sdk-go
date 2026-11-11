package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// CanonicalQuery sorts URL-decoded query parameters by key, then value, before
// RFC 3986 encoding, matching the server's API-key canonicalization.
func CanonicalQuery(rawURL string) (string, error) {
	parsed, err := parseSigningURL(rawURL)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		sort.Strings(values[key])
		for _, value := range values[key] {
			pairs = append(pairs, queryComponentEscape(key)+"="+queryComponentEscape(value))
		}
	}
	return strings.Join(pairs, "&"), nil
}

// Bounds of an API-key request's receive window (X-API-RECV-WINDOW), in
// milliseconds. Without the header the server applies 5000.
const (
	MinRecvWindowMS = 1
	MaxRecvWindowMS = 60000
)

// ValidateRecvWindowMS checks a receive window before anything is signed: 0
// means no window header, and any other value must be from MinRecvWindowMS to
// MaxRecvWindowMS. The server refuses a header outside that range as
// AUTH_RECV_WINDOW_INVALID.
func ValidateRecvWindowMS(recvWindowMS int) error {
	if recvWindowMS != 0 && (recvWindowMS < MinRecvWindowMS || recvWindowMS > MaxRecvWindowMS) {
		return &sdkerrors.ValidationError{Msg: "recv window must be a whole number of milliseconds from 1 to 60000, or 0 for none"}
	}
	return nil
}

// CanonicalSigningString builds the Polyester API-key signing payload.
func CanonicalSigningString(timestampMS, method, rawURL string, body []byte) (string, error) {
	return canonicalSigningString(timestampMS, "", method, rawURL, body)
}

// CanonicalSigningStringWithRecvWindow builds the signing payload of a request
// that sends X-API-RECV-WINDOW: the window's decimal string is its own line,
// right after the timestamp. recvWindowMS 0 builds CanonicalSigningString's
// payload, for a request without the header.
func CanonicalSigningStringWithRecvWindow(timestampMS string, recvWindowMS int, method, rawURL string, body []byte) (string, error) {
	if err := ValidateRecvWindowMS(recvWindowMS); err != nil {
		return "", err
	}
	return canonicalSigningString(timestampMS, recvWindowHeader(recvWindowMS), method, rawURL, body)
}

func recvWindowHeader(recvWindowMS int) string {
	if recvWindowMS == 0 {
		return ""
	}
	return strconv.Itoa(recvWindowMS)
}

func canonicalSigningString(timestampMS, recvWindow, method, rawURL string, body []byte) (string, error) {
	parsed, err := parseSigningURL(rawURL)
	if err != nil {
		return "", err
	}
	pathname := "/"
	if parsed.Path != "" {
		pathname = parsed.Path
	}
	query, err := CanonicalQuery(rawURL)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	lines := []string{timestampMS}
	if recvWindow != "" {
		lines = append(lines, recvWindow)
	}
	lines = append(lines, strings.ToUpper(method), pathname, query, hex.EncodeToString(sum[:]))
	return strings.Join(lines, "\n"), nil
}

// SignRequest returns Polyester API-key headers for an HTTP request.
func SignRequest(creds *Credentials, method, rawURL string, body []byte, timestampMS string) (map[string]string, error) {
	return SignRequestWithRecvWindow(creds, method, rawURL, body, timestampMS, 0)
}

// SignRequestWithRecvWindow returns Polyester API-key headers for an HTTP
// request with a receive window: the server processes the request only until
// its timestamp plus recvWindowMS milliseconds, and refuses it after that as
// AUTH_TIMESTAMP_SKEW, or ERROR_CODE_REQUEST_DEADLINE_EXCEEDED once
// authenticated. The headers then include X-API-RECV-WINDOW, which the
// signature covers. recvWindowMS 0 signs as SignRequest does, without the
// header; a value outside ValidateRecvWindowMS's range is refused before
// anything is signed.
func SignRequestWithRecvWindow(creds *Credentials, method, rawURL string, body []byte, timestampMS string, recvWindowMS int) (map[string]string, error) {
	if err := ValidateRecvWindowMS(recvWindowMS); err != nil {
		return nil, err
	}
	if timestampMS == "" {
		var err error
		timestampMS, err = automaticSigningTimestamp(creds)
		if err != nil {
			return nil, err
		}
	}
	recvWindow := recvWindowHeader(recvWindowMS)
	canonical, err := canonicalSigningString(timestampMS, recvWindow, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	private := creds.privateKey()
	if len(private) != ed25519.PrivateKeySize {
		return nil, &sdkerrors.AuthError{Msg: "valid API-key credentials are required to sign requests"}
	}
	sig := ed25519.Sign(private, []byte(canonical))
	headers := map[string]string{
		"X-API-KEY-ID":    creds.KeyID,
		"X-API-TIMESTAMP": timestampMS,
		"X-API-SIGNATURE": hex.EncodeToString(sig),
	}
	if recvWindow != "" {
		headers["X-API-RECV-WINDOW"] = recvWindow
	}
	return headers, nil
}

func parseSigningURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, &sdkerrors.AuthError{Msg: "cannot sign malformed absolute HTTP URL"}
	}
	return parsed, nil
}

// queryComponentEscape matches Python urllib.parse.quote(..., safe="").
func queryComponentEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// RequestURL joins apiBase with a Connect procedure path.
func RequestURL(apiBase, procedure string) string {
	base := strings.TrimRight(apiBase, "/")
	proc := procedure
	if !strings.HasPrefix(proc, "/") {
		proc = "/" + proc
	}
	return base + proc
}
