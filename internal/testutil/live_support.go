package testutil

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// CallOptions configures optional-route integration helpers.
type CallOptions struct {
	AllowProtoMismatch bool
	AllowJWTOnly       bool
}

func defaultCallOptions() CallOptions {
	return CallOptions{AllowProtoMismatch: true, AllowJWTOnly: true}
}

// RouteUnavailable reports whether devnet does not mount the RPC.
func RouteUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var route *sdkerrors.RouteNotFoundError
	if errors.As(err, &route) {
		return true
	}
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		code := strings.ToLower(strings.TrimSpace(api.Code))
		switch code {
		case "route_not_found", "unimplemented", "not_found":
			return true
		}
	}
	return false
}

// DevnetUnavailable reports transient devnet outages that should skip optional tests.
func DevnetUnavailable(err error) bool {
	if DevnetProtoMismatch(err) {
		return true
	}
	var srv *sdkerrors.ServerError
	if errors.As(err, &srv) {
		msg := strings.ToLower(srv.Msg)
		return strings.Contains(msg, "temporarily unavailable") || strings.Contains(msg, "unavailable")
	}
	return false
}

// DevnetProtoMismatch reports likely proto/version skew on devnet.
func DevnetProtoMismatch(err error) bool {
	var srv *sdkerrors.ServerError
	if !errors.As(err, &srv) {
		return false
	}
	msg := strings.ToLower(srv.Msg)
	return strings.Contains(msg, "internal error") || strings.Contains(msg, "decode") || strings.Contains(msg, "proto")
}

// IsPermissionDenied reports structured Auth/403 permission failures (F-24).
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	permissionish := strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "permission_denied") ||
		strings.Contains(msg, "http 403")

	var auth *sdkerrors.AuthError
	if errors.As(err, &auth) {
		return permissionish || strings.EqualFold(auth.Code, "permission_denied")
	}
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		code := strings.ToLower(strings.TrimSpace(api.Code))
		return permissionish || strings.Contains(code, "permission_denied")
	}
	return false
}

// JWTSessionOnly reports auth errors that indicate JWT/session-only routes.
func JWTSessionOnly(err error) bool {
	if err == nil {
		return false
	}
	// Permission-denied is classified separately (F-24).
	if IsPermissionDenied(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
    sessionish := strings.Contains(msg, "authorization header") ||
		strings.Contains(msg, "bearer") ||
		strings.Contains(msg, "interactive session") ||
		strings.Contains(msg, "api keys cannot")

	var auth *sdkerrors.AuthError
	if errors.As(err, &auth) {
		return sessionish
	}
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		code := strings.ToLower(strings.TrimSpace(api.Code))
		return sessionish || code == "unauthenticated"
	}
	return false
}

// CallOptional runs a live RPC and skips when the route is unavailable on devnet.
func CallOptional[T any](t *testing.T, label string, fn func() (T, error), opts ...CallOptions) T {
	t.Helper()
	o := defaultCallOptions()
	if len(opts) > 0 {
		o = opts[0]
	}
	v, err := fn()
	if err == nil {
		return v
	}
	if RouteUnavailable(err) {
		SoftSkipf(t, "%s not mounted on devnet: %v", label, err)
	}
	if IsPermissionDenied(err) {
		SoftSkipf(t, "%s missing required API-key permission (declare fixture scopes): %v", label, err)
	}
	if o.AllowJWTOnly && JWTSessionOnly(err) {
		SoftSkipf(t, "%s requires JWT/session auth (API key not accepted on devnet): %v", label, err)
	}
	if o.AllowProtoMismatch && (DevnetProtoMismatch(err) || DevnetUnavailable(err)) {
		SoftSkipf(t, "%s unavailable on devnet: %v", label, err)
	}
	t.Fatalf("%s: %v", label, err)
	var zero T
	return zero
}

// CallRequired runs a live RPC that must exist on devnet.
func CallRequired[T any](t *testing.T, label string, fn func() (T, error)) T {
	t.Helper()
	v, err := fn()
	if err == nil {
		return v
	}
	if RouteUnavailable(err) {
		t.Fatalf("%s returned route not found on devnet: %v", label, err)
	}
	t.Fatalf("%s: %v", label, err)
	var zero T
	return zero
}

// AssertAPIDataShape checks required keys in an ApiData-style raw map.
func AssertAPIDataShape(t *testing.T, raw map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := raw[key]; !ok {
			t.Fatalf("expected response key %q, got keys %v", key, mapKeys(raw))
		}
	}
}

func mapKeys(raw map[string]any) []string {
	out := make([]string, 0, len(raw))
	for k := range raw {
		out = append(out, k)
	}
	return out
}
