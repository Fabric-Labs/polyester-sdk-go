package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// CanonicalQuery sorts and URL-encodes query parameters from rawURL.
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
		for _, value := range values[key] {
			pairs = append(pairs, queryComponentEscape(key)+"="+queryComponentEscape(value))
		}
	}
	return strings.Join(pairs, "&"), nil
}

// CanonicalSigningString builds the Polyester API-key signing payload.
func CanonicalSigningString(timestampMS, method, rawURL string, body []byte) (string, error) {
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
	return strings.Join([]string{
		timestampMS,
		strings.ToUpper(method),
		pathname,
		query,
		hex.EncodeToString(sum[:]),
	}, "\n"), nil
}

// SignRequest returns Polyester API-key headers for an HTTP request.
func SignRequest(creds *Credentials, method, rawURL string, body []byte, timestampMS string) (map[string]string, error) {
	if timestampMS == "" {
		timestampMS = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	canonical, err := CanonicalSigningString(timestampMS, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(creds.PrivateKey, []byte(canonical))
	return map[string]string{
		"X-API-KEY-ID":    creds.KeyID,
		"X-API-TIMESTAMP": timestampMS,
		"X-API-SIGNATURE": hex.EncodeToString(sig),
	}, nil
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
