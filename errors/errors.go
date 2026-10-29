package errors

import (
	"errors"
	"strconv"
)

// ErrPolyester is the root sentinel for SDK errors.
var ErrPolyester = errors.New("polyester")

// Stable auth.v1.AuthErrorDetail codes used for MFA control flow and retries.
// Prefer these over ConnectError message text.
const (
	AuthCodeMFANotEnrolled        = "AUTH_MFA_NOT_ENROLLED"
	AuthCodeStepUpRequired        = "AUTH_STEP_UP_REQUIRED"
	AuthCodeMFAElevationRequired  = "AUTH_MFA_ELEVATION_REQUIRED"
	AuthCodeMFALastFactorRequired = "AUTH_MFA_LAST_FACTOR_REQUIRED"
	AuthCodeInternalError         = "AUTH_INTERNAL_ERROR"
	// AuthCodeTemporarilyUnavailable: the request changed nothing; retry the
	// same request.
	AuthCodeTemporarilyUnavailable = "AUTH_TEMPORARILY_UNAVAILABLE"
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

// AuthError indicates missing or invalid credentials / permissions.
type AuthError struct {
	Msg      string
	Status   int    // HTTP status when sourced from an HTTP exchange (e.g. 401/403)
	Label    string // backward-compatible endpoint/procedure label
	Body     string // truncated server body
	Code     string // stable server code, when supplied
	Context  string // channel/procedure context
	Endpoint string // exact HTTP endpoint used
}

func (e *AuthError) Error() string {
	if e == nil {
		return "auth error"
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Status != 0 && e.Label != "" {
		return e.Label + ": HTTP " + strconv.Itoa(e.Status)
	}
	return "auth error"
}
func (e *AuthError) Is(target error) bool { return target == ErrPolyester }

// ValidationError indicates invalid SDK input or a concrete server-side
// validation rejection. Code and Metadata are populated when structured wire
// details are available.
type ValidationError struct {
	Msg      string
	Code     string
	Metadata map[string]string
}

func (e *ValidationError) Error() string        { return e.Msg }
func (e *ValidationError) Is(target error) bool { return target == ErrPolyester }

// TransportError indicates network or timeout failures.
type TransportError struct{ Msg string }

func (e *TransportError) Error() string        { return e.Msg }
func (e *TransportError) Is(target error) bool { return target == ErrPolyester }

// RateLimitError indicates rate limiting from the API.
//
// Detail carries structured polyester.ratelimit.v1.RateLimitDetail when the
// server attached it (Connect details or in-band order rejection). RetryAfter
// prefers detail.retry_after_ms, then response headers.
type RateLimitError struct {
	Msg        string
	RetryAfter *float64
	Detail     *RateLimitDetail
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

// ResponseContractError indicates that a successful mutation response violated
// the documented wire contract. The mutation may already have been applied, so
// callers must reconcile state rather than blindly retrying.
type ResponseContractError struct {
	Operation string
	Msg       string
}

func (e *ResponseContractError) Error() string {
	if e == nil {
		return "response contract violation"
	}
	if e.Operation == "" {
		return "response contract violation: " + e.Msg
	}
	return e.Operation + " response contract violation: " + e.Msg
}
func (e *ResponseContractError) Is(target error) bool { return target == ErrPolyester }

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
//
// On the private order stream, Code names the server's request error (for
// example "FORBIDDEN" or "TOO_MANY_SUBSCRIPTIONS") and CloseCode is the
// WebSocket close code that ended the stream (for example 4403). Both are
// empty for other realtime failures.
type RealtimeError struct {
	Msg       string
	Code      string
	CloseCode int
}

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

// IsRetryable reports whether retrying may succeed after backoff.
// AUTH_TEMPORARILY_UNAVAILABLE and ERROR_CODE_ACCOUNT_REPAIRING changed
// nothing: retry the same request.
//
// It does not guarantee that a mutation was not applied. Preserve the same
// idempotency key and reconcile first when MutationOutcomeUnknown is true.
func IsRetryable(err error) bool {
	var transportErr *TransportError
	var rateLimitErr *RateLimitError
	var serverErr *ServerError
	code := apiErrorCode(err)
	return errors.As(err, &transportErr) ||
		errors.As(err, &rateLimitErr) ||
		errors.As(err, &serverErr) ||
		outcomeUnknownCodes[code] ||
		code == "ERROR_CODE_ACCOUNT_REPAIRING" ||
		code == AuthCodeTemporarilyUnavailable
}

// MutationOutcomeUnknown reports whether the server may have applied a failed
// mutation before the error became visible to the caller. It includes an
// order create, cancel or modify and a trigger create that was sent but not
// confirmed in time (ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN,
// ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN, ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN,
// ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN): read the order or trigger back
// before retrying.
func MutationOutcomeUnknown(err error) bool {
	var transportErr *TransportError
	var serverErr *ServerError
	var contractErr *ResponseContractError
	return errors.As(err, &transportErr) ||
		errors.As(err, &serverErr) ||
		errors.As(err, &contractErr) ||
		outcomeUnknownCodes[apiErrorCode(err)]
}

// outcomeUnknownCodes are order error codes for a mutation that was sent but
// not confirmed in time; it may still apply.
var outcomeUnknownCodes = map[string]bool{
	"ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN":   true,
	"ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN":   true,
	"ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN":   true,
	"ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN": true,
}

func apiErrorCode(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}
