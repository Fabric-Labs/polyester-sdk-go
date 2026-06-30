package errors

import "errors"

// ErrPolyester is the root sentinel for SDK errors.
var ErrPolyester = errors.New("polyester")

// AuthError indicates missing or invalid credentials.
type AuthError struct{ Msg string }

func (e *AuthError) Error() string        { return e.Msg }
func (e *AuthError) Is(target error) bool { return target == ErrPolyester }

// ValidationError indicates invalid SDK input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string        { return e.Msg }
func (e *ValidationError) Is(target error) bool { return target == ErrPolyester }

// TransportError indicates network or timeout failures.
type TransportError struct{ Msg string }

func (e *TransportError) Error() string        { return e.Msg }
func (e *TransportError) Is(target error) bool { return target == ErrPolyester }

// RateLimitError indicates rate limiting from the API.
type RateLimitError struct {
	Msg        string
	RetryAfter *float64
}

func (e *RateLimitError) Error() string        { return e.Msg }
func (e *RateLimitError) Is(target error) bool { return target == ErrPolyester }

// ServerError indicates a server-side 5xx failure.
type ServerError struct{ Msg string }

func (e *ServerError) Error() string        { return e.Msg }
func (e *ServerError) Is(target error) bool { return target == ErrPolyester }

// APIError indicates a structured Connect/API error.
type APIError struct {
	Msg      string
	Code     string
	Metadata map[string]string
}

func (e *APIError) Error() string        { return e.Msg }
func (e *APIError) Is(target error) bool { return target == ErrPolyester }

// RouteNotFoundError indicates the gateway has no route for an RPC.
type RouteNotFoundError struct{ Procedure string }

func (e *RouteNotFoundError) Error() string {
	hint := "RPC not exposed on this API host"
	if e.Procedure != "" {
		hint += ": " + e.Procedure
	}
	return hint + ". The procedure may be unimplemented on devnet or disabled in this environment."
}

func (e *RouteNotFoundError) Is(target error) bool { return target == ErrPolyester }

// RealtimeError indicates realtime connection or decode failures.
type RealtimeError struct{ Msg string }

func (e *RealtimeError) Error() string        { return e.Msg }
func (e *RealtimeError) Is(target error) bool { return target == ErrPolyester }
