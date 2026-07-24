package errors

import "errors"

// ErrPolyester is the root sentinel for SDK errors.
var ErrPolyester = errors.New("polyester")

// Stable auth.v1.AuthErrorDetail codes used for MFA control flow.
// Prefer these over ConnectError message text.
const (
	AuthCodeMFANotEnrolled        = "AUTH_MFA_NOT_ENROLLED"
	AuthCodeStepUpRequired        = "AUTH_STEP_UP_REQUIRED"
	AuthCodeMFAElevationRequired  = "AUTH_MFA_ELEVATION_REQUIRED"
	AuthCodeMFALastFactorRequired = "AUTH_MFA_LAST_FACTOR_REQUIRED"
)

// AuthErrorCode returns the structured auth.v1.AuthErrorDetail code when err is
// an *APIError that carries one. Otherwise it returns "".
func AuthErrorCode(err error) string {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	return apiErr.Code
}

// IsMFAEnrollmentRequired reports whether err requires MFA enrollment before
// the operation can continue (AUTH_MFA_NOT_ENROLLED).
func IsMFAEnrollmentRequired(err error) bool {
	return AuthErrorCode(err) == AuthCodeMFANotEnrolled
}

// IsStepUpRequired reports whether err requires a fresh one-use step-up proof
// via X-Auth-Step-Up (AUTH_STEP_UP_REQUIRED).
func IsStepUpRequired(err error) bool {
	return AuthErrorCode(err) == AuthCodeStepUpRequired
}

// IsMFAElevationRequired reports whether err requires a recent MFA-elevated
// interactive session, not a one-use step-up proof (AUTH_MFA_ELEVATION_REQUIRED).
func IsMFAElevationRequired(err error) bool {
	return AuthErrorCode(err) == AuthCodeMFAElevationRequired
}

// IsMFALastFactorRequired reports whether err rejects removing the final
// active MFA factor (AUTH_MFA_LAST_FACTOR_REQUIRED).
func IsMFALastFactorRequired(err error) bool {
	return AuthErrorCode(err) == AuthCodeMFALastFactorRequired
}

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

// QueueOverflowError indicates a realtime subscription discarded work because the
// consumer lagged behind the publication rate. Subscriptions fail instead of
// silently dropping updates.
type QueueOverflowError struct{ Msg string }

func (e *QueueOverflowError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "realtime subscription queue full; consumer too slow"
}
func (e *QueueOverflowError) Is(target error) bool { return target == ErrPolyester }
