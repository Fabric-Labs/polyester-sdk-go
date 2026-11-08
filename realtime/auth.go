package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
)

const (
	maxTokenResponseBytes = 64 * 1024
	maxAuthErrorBodyBytes = 512
)

func connectionTokenURL(apiURL string) string {
	return strings.TrimRight(apiURL, "/") + "/v1/rt/token"
}

func subscriptionTokenURL(apiURL, channel string) string {
	base := strings.TrimRight(apiURL, "/") + "/v1/rt/subscribe"
	return base + "?channel=" + url.QueryEscape(channel)
}

func contentLengthExceedsLimit(header http.Header, maxBytes int) bool {
	raw := header.Get("Content-Length")
	if raw == "" {
		return false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return false
	}
	return n > maxBytes
}

func truncateAuthBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) <= maxAuthErrorBodyBytes {
		return text
	}
	return text[:maxAuthErrorBodyBytes] + "…"
}

func authHTTPError(status int, label string, body []byte) error {
	truncated := truncateAuthBody(body)
	problem := sdkerrors.DecodeHTTPProblem(body, status)
	code := ""
	serverMessage := ""
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		if value, ok := payload["code"].(string); ok {
			code = strings.TrimSpace(value)
		}
		if value, ok := payload["message"].(string); ok {
			serverMessage = strings.TrimSpace(value)
		}
	}
	if code == "" {
		if status == http.StatusForbidden {
			code = "permission_denied"
		} else {
			code = "unauthenticated"
		}
	}
	detail := serverMessage
	if problem != nil {
		code = problem.Code
		detail = problem.Detail
	}
	if detail == "" && problem == nil {
		detail = truncated
	}
	msg := fmt.Sprintf("%s: HTTP %d, code %s", label, status, code)
	if detail != "" {
		msg += ": " + detail
	}
	return &sdkerrors.AuthError{
		Msg:     msg,
		Problem: problem,
		Status:  status,
		Label:   label,
		Body:    truncated,
		Code:    code,
		Context: label,
	}
}

// signedGet performs one API-key-signed GET and returns a successful body.
// 401 and 403 become AuthError; other failures are realtime or transport errors.
func signedGet(ctx context.Context, httpClient *http.Client, creds *auth.Credentials, rawURL, label string, maxBytes int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, redact.URLError(err)
	}
	headers, err := auth.SignRequest(creds, http.MethodGet, rawURL, nil, "")
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, redact.URLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if contentLengthExceedsLimit(resp.Header, maxBytes) {
		return nil, &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: response exceeds %d bytes", label, maxBytes)}
	}
	limited := io.LimitReader(resp.Body, int64(maxBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, &sdkerrors.RealtimeError{Msg: label + ": read body: " + err.Error()}
	}
	if len(body) > maxBytes {
		return nil, &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: response exceeds %d bytes", label, maxBytes)}
	}
	switch resp.StatusCode {
	case http.StatusForbidden:
		if useragent.IsCloudflareBrowserBan(string(body)) {
			return nil, &sdkerrors.TransportError{Msg: useragent.Cloudflare1010Message()}
		}
		fallthrough
	case http.StatusUnauthorized:
		authErr := authHTTPError(resp.StatusCode, label, body).(*sdkerrors.AuthError)
		endpoint := redact.URL(rawURL)
		authErr.Endpoint = endpoint
		authErr.Msg += " [" + endpoint + "]"
		return nil, authErr
	}
	if resp.StatusCode >= 400 {
		return nil, &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: HTTP %d", label, resp.StatusCode)}
	}
	return body, nil
}

func fetchRTToken(ctx context.Context, httpClient *http.Client, creds *auth.Credentials, rawURL, label string) (string, error) {
	body, err := signedGet(ctx, httpClient, creds, rawURL, label, maxTokenResponseBytes)
	if err != nil {
		return "", err
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", &sdkerrors.RealtimeError{Msg: label + ": invalid token response"}
	}
	if payload.Token == "" {
		return "", &sdkerrors.RealtimeError{Msg: label + ": response missing token"}
	}
	return payload.Token, nil
}

func fetchConnectionToken(ctx context.Context, httpClient *http.Client, creds *auth.Credentials, apiURL string) (string, error) {
	return fetchRTToken(ctx, httpClient, creds, connectionTokenURL(apiURL), "realtime connection token")
}

func fetchSubscriptionToken(ctx context.Context, httpClient *http.Client, creds *auth.Credentials, apiURL, channel string) (string, error) {
	return fetchRTToken(ctx, httpClient, creds, subscriptionTokenURL(apiURL, channel), "realtime subscription token for "+channel)
}
