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
)

const maxTokenResponseBytes = 64 * 1024

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

func fetchRTToken(ctx context.Context, httpClient *http.Client, creds *auth.Credentials, rawURL, label string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	for key, value := range auth.SignRequest(creds, http.MethodGet, rawURL, nil, "") {
		req.Header.Set(key, value)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if contentLengthExceedsLimit(resp.Header, maxTokenResponseBytes) {
		return "", &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: response exceeds %d bytes", label, maxTokenResponseBytes)}
	}
	limited := io.LimitReader(resp.Body, int64(maxTokenResponseBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", &sdkerrors.RealtimeError{Msg: label + ": read body: " + err.Error()}
	}
	if len(body) > maxTokenResponseBytes {
		return "", &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: response exceeds %d bytes", label, maxTokenResponseBytes)}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "", &sdkerrors.AuthError{Msg: label + ": authentication failed"}
	}
	if resp.StatusCode >= 400 {
		return "", &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s: HTTP %d", label, resp.StatusCode)}
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
