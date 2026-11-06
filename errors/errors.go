package errors

import (
	"errors"
	"strconv"
	"strings"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
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
	// AuthCodeSubaccountOwnerRoleLocked: a member call targeted the
	// sub-account's owner. See SubaccountOwnerRoleLockedError.
	AuthCodeSubaccountOwnerRoleLocked = "AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED"
	// AuthCodeAuthenticationRequired: the request carried no credentials, or
	// a missing, malformed or expired session token (REST UNAUTHENTICATED).
	AuthCodeAuthenticationRequired = "AUTH_AUTHENTICATION_REQUIRED"
	// AuthCodeSessionKindNotAllowed: the method accepts only a session token
	// and the request carried an account API key. See SessionRequiredError.
	AuthCodeSessionKindNotAllowed = "AUTH_SESSION_KIND_NOT_ALLOWED"
)

// API-key refusals (auth.v1.AuthErrorCode). Order reads, order and trigger
// writes and balance reads answer them on Connect as unauthenticated with an
// auth.v1.AuthErrorDetail; REST problem bodies name them without the AUTH_
// prefix. Only AuthCodeTimestampSkew is retryable, as TimestampSkewError; the
// others need a change by the caller.
const (
	// AuthCodeSignatureInvalid: the signature does not verify, or the key is unknown.
	AuthCodeSignatureInvalid = "AUTH_SIGNATURE_INVALID"
	// AuthCodeTimestampSkew: the request timestamp is malformed or too far
	// from the server clock. Maps to TimestampSkewError.
	AuthCodeTimestampSkew = "AUTH_TIMESTAMP_SKEW"
	// AuthCodeMissingHeaders: the timestamp or signature header is missing.
	AuthCodeMissingHeaders = "AUTH_MISSING_HEADERS"
	// AuthCodeKeyExpired: the API key has expired.
	AuthCodeKeyExpired = "AUTH_KEY_EXPIRED"
	// AuthCodeKeyDisabled: the API key is disabled or revoked.
	AuthCodeKeyDisabled = "AUTH_KEY_DISABLED"
	// AuthCodeIPNotAllowed: the key's IP allowlist does not include the
	// request's source address.
	AuthCodeIPNotAllowed = "AUTH_IP_NOT_ALLOWED"
	// AuthCodeAPIKeyNotAccepted: the API does not accept API keys.
	AuthCodeAPIKeyNotAccepted = "AUTH_API_KEY_NOT_ACCEPTED"
)

// AuthErrorCode returns the auth.v1.AuthErrorCode name of err, such as
// "AUTH_SIGNATURE_INVALID", or "" when err carries no auth reason. It reads
// the Connect auth.v1.AuthErrorDetail code and the REST problem code alike: a
// REST code gains the AUTH_ prefix ("IP_NOT_ALLOWED" reads
// "AUTH_IP_NOT_ALLOWED"), and REST "UNAUTHENTICATED" reads
// "AUTH_AUTHENTICATION_REQUIRED". A TimestampSkewError reads
// "AUTH_TIMESTAMP_SKEW" and a SessionRequiredError
// "AUTH_SESSION_KIND_NOT_ALLOWED". Compare with the AuthCode* constants.
func AuthErrorCode(err error) string {
	var skewErr *TimestampSkewError
	if errors.As(err, &skewErr) {
		return AuthCodeTimestampSkew
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if code := authCodeName(apiErr.Code); code != "" {
			return code
		}
	}
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authCodeName(authErr.Code)
	}
	return ""
}

// authCodeName names code as an auth.v1.AuthErrorCode: an AUTH_ code as sent,
// a REST code with the AUTH_ prefix when the contract knows it.
func authCodeName(code string) string {
	switch {
	case code == "UNAUTHENTICATED":
		return AuthCodeAuthenticationRequired
	case strings.HasPrefix(code, "AUTH_"):
		return code
	}
	if _, ok := authv1.AuthErrorCode_value["AUTH_"+code]; ok && code != "" && code != "UNSPECIFIED" {
		return "AUTH_" + code
	}
	return ""
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
	Problem  *HTTPProblem // REST metadata, when supplied by the server.
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

// SubaccountOwnerRoleLockedError indicates that a sub-account member call
// targeted the sub-account's owner (AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED): the
// owner cannot be invited, have their role changed or be removed. The request
// changed nothing, and retrying does not help. Show "The owner's role can't be
// changed", and don't offer these actions for the member whose role is OWNER.
//
// It unwraps to an *APIError with that code, so AuthErrorCode reads it.
type SubaccountOwnerRoleLockedError struct {
	Problem *HTTPProblem // REST metadata, when supplied by the server.
	Msg     string
	Status  int // HTTP status when sourced from a REST problem body (412)
}

func (e *SubaccountOwnerRoleLockedError) Error() string {
	if e == nil || e.Msg == "" {
		return "the sub-account owner's role can't be changed"
	}
	return e.Msg
}
func (e *SubaccountOwnerRoleLockedError) Is(target error) bool { return target == ErrPolyester }
func (e *SubaccountOwnerRoleLockedError) Unwrap() error {
	return &APIError{Msg: e.Error(), Code: AuthCodeSubaccountOwnerRoleLocked}
}

// IsSubaccountOwnerRoleLocked reports whether err refuses a member call that
// targeted the sub-account's owner (AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED).
func IsSubaccountOwnerRoleLocked(err error) bool {
	var locked *SubaccountOwnerRoleLockedError
	return errors.As(err, &locked)
}

// SessionRequiredError indicates that the method accepts only an interactive
// session (a session token) and the request carried an account API key
// (AUTH_SESSION_KIND_NOT_ALLOWED: Connect permission_denied with an
// auth.v1.AuthErrorDetail, REST 403). Every session-only method answers it
// the same way, for example withdraw, auto-withdraw rule writes, portfolio
// equity reads and VIP status. The request changed nothing, and retrying with
// the same credentials does not help: this SDK signs with API keys, so call
// the method from a signed-in session instead.
//
// It unwraps to an *APIError with that code, so AuthErrorCode reads it.
type SessionRequiredError struct {
	Problem *HTTPProblem // REST metadata, when supplied by the server.
	Msg     string
	Status  int // HTTP status when sourced from a REST problem body (403)
}

func (e *SessionRequiredError) Error() string {
	if e == nil || e.Msg == "" {
		return "interactive session required"
	}
	return e.Msg
}
func (e *SessionRequiredError) Is(target error) bool { return target == ErrPolyester }
func (e *SessionRequiredError) Unwrap() error {
	return &APIError{Msg: e.Error(), Code: AuthCodeSessionKindNotAllowed}
}

// IsSessionRequired reports whether err refuses an API key on a method that
// accepts only a session token (AUTH_SESSION_KIND_NOT_ALLOWED).
func IsSessionRequired(err error) bool {
	var required *SessionRequiredError
	return errors.As(err, &required)
}

// FieldViolation names one request field a client must fix. FieldPath is the
// path of the transport that answered: Connect names proto fields
// ("order.base_qty_scaled", "items[1].new_attached_risk.trailing_stop.limit_price_ticks"),
// REST names its JSON fields ("order.baseQty"). Branch on the error code, and
// on RuleID within it; Message is human-readable only.
type FieldViolation struct {
	FieldPath string `json:"field_path,omitempty"`
	RuleID    string `json:"rule_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ValidationError indicates invalid SDK input or a concrete server-side
// validation rejection. Code and Metadata are populated when structured wire
// details are available. Violations lists the fields to fix, from an order
// ErrorDetail, buf.validate violations, a REST problem body or the SDK's own
// checks; Metadata carries the same violations as violation.N.field_path,
// violation.N.rule_id and violation.N.message.
type ValidationError struct {
	Problem    *HTTPProblem // REST metadata, when supplied by the server.
	Msg        string
	Code       string
	Metadata   map[string]string
	Violations []FieldViolation
}

func (e *ValidationError) Error() string        { return e.Msg }
func (e *ValidationError) Is(target error) bool { return target == ErrPolyester }

// Violations returns the field violations err carries, or nil.
func Violations(err error) []FieldViolation {
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Violations) == 0 {
		return nil
	}
	return append([]FieldViolation(nil), validationErr.Violations...)
}

// ViolationMetadata renders violations as violation.N.field_path,
// violation.N.rule_id and violation.N.message entries.
func ViolationMetadata(violations []FieldViolation) map[string]string {
	if len(violations) == 0 {
		return nil
	}
	metadata := make(map[string]string, len(violations)*3)
	for i, violation := range violations {
		prefix := "violation." + strconv.Itoa(i) + "."
		metadata[prefix+"field_path"] = violation.FieldPath
		metadata[prefix+"rule_id"] = violation.RuleID
		metadata[prefix+"message"] = violation.Message
	}
	return metadata
}

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
	Code       string // Structured service code; empty when none was supplied. Known order codes use ERROR_CODE_.
	Msg        string
	RetryAfter *float64
	Detail     *RateLimitDetail
}

func (e *RateLimitError) Error() string        { return e.Msg }
func (e *RateLimitError) Is(target error) bool { return target == ErrPolyester }

// ServerError indicates a server-side 5xx failure.
type ServerError struct {
	Msg    string
	Code   string // Structured service code, when supplied.
	Status int    // HTTP status corresponding to the failure, when known.
}

func (e *ServerError) Error() string        { return e.Msg }
func (e *ServerError) Is(target error) bool { return target == ErrPolyester }

// APIError indicates a structured Connect/API error.
type APIError struct {
	Problem  *HTTPProblem // REST metadata for outcome-unknown and ORDER_SEQUENCE_EXHAUSTED problems.
	Msg      string
	Code     string
	Metadata map[string]string
	// ExistingOrderID is the order already holding the client order ID on an
	// ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID rejection, when the server
	// knows it. Read it with Orders.Get for its current state.
	ExistingOrderID string
}

func (e *APIError) Error() string        { return e.Msg }
func (e *APIError) Is(target error) bool { return target == ErrPolyester }

// TimestampSkewError indicates that the API-key signature timestamp fell
// outside the server's skew window (TIMESTAMP_SKEW), for example because the
// request waited too long in a large concurrent burst or the local clock
// drifted. The request changed nothing: retrying signs it again with a fresh
// timestamp. Bound concurrency or correct the clock if it recurs.
type TimestampSkewError struct {
	Problem *HTTPProblem // REST metadata, when supplied by the server.
	Msg     string
	Status  int // HTTP status, usually 401
}

func (e *TimestampSkewError) Error() string        { return e.Msg }
func (e *TimestampSkewError) Is(target error) bool { return target == ErrPolyester }

// NotImplementedError indicates that the server does not implement the
// requested operation (Connect unimplemented or HTTP 501). Retrying does not
// help.
type NotImplementedError struct{ Msg string }

func (e *NotImplementedError) Error() string        { return e.Msg }
func (e *NotImplementedError) Is(target error) bool { return target == ErrPolyester }

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

// BatchReplaceWindowExpiredError indicates that a batch replace's 60-second
// status window ended while an item still read admitted. After the window, a
// status read proves nothing (not found does not mean rejected): reconcile the
// orders themselves with order reads, keyed by each item's old order ID.
type BatchReplaceWindowExpiredError struct {
	BatchRequestID string
}

func (e *BatchReplaceWindowExpiredError) Error() string {
	return "batch replace " + e.BatchRequestID +
		" still has admitted items at the end of its 60 s status window; reconcile its orders with order reads"
}
func (e *BatchReplaceWindowExpiredError) Is(target error) bool { return target == ErrPolyester }

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
// AUTH_TEMPORARILY_UNAVAILABLE, ERROR_CODE_ACCOUNT_REPAIRING and
// TimestampSkewError changed nothing: retry the same request.
//
// It does not guarantee that a mutation was not applied. Preserve the same
// idempotency key and reconcile first when MutationOutcomeUnknown is true.
// A REST gateway 504 is non-retryable with outcome unknown even without a
// recognized code. This classifier has no operation context: safe reads may
// explicitly retry it; writes must read back first. A client order ID enables
// reconciliation, not exact replay of a single create.
// An order error code ending in _OUTCOME_UNKNOWN is not retryable: read the
// order or trigger back before sending it again.
func IsRetryable(err error) bool {
	var transportErr *TransportError
	var rateLimitErr *RateLimitError
	var serverErr *ServerError
	var skewErr *TimestampSkewError
	code := apiErrorCode(err)
	if isOutcomeUnknownCode(code) || (errors.As(err, &serverErr) && serverErr.Status == 504) {
		return false
	}
	return errors.As(err, &transportErr) ||
		errors.As(err, &rateLimitErr) ||
		errors.As(err, &serverErr) ||
		errors.As(err, &skewErr) ||
		code == "ERROR_CODE_ACCOUNT_REPAIRING" ||
		code == AuthCodeTemporarilyUnavailable
}

// MutationOutcomeUnknown reports whether the server may have applied a failed
// mutation before the error became visible to the caller. It includes every
// order error code ending in _OUTCOME_UNKNOWN: an order create, cancel or
// modify, or a trigger create, cancel, pause, resume or modify, that was sent
// but not confirmed in time. Read the order or trigger back before retrying.
func MutationOutcomeUnknown(err error) bool {
	var transportErr *TransportError
	var serverErr *ServerError
	var contractErr *ResponseContractError
	return errors.As(err, &transportErr) ||
		errors.As(err, &serverErr) ||
		errors.As(err, &contractErr) ||
		isOutcomeUnknownCode(apiErrorCode(err))
}

// isOutcomeUnknownCode recognises a mutation outcome by its code suffix,
// including codes introduced by a newer server before SDK regeneration.
func isOutcomeUnknownCode(code string) bool {
	return strings.HasSuffix(code, "_OUTCOME_UNKNOWN")
}

// IsOrderAlreadyTerminal reports whether a cancel (or another order change)
// was refused because the order has already ended
// (ERROR_CODE_ORDER_ALREADY_TERMINAL): filled, expired or canceled, by this
// cancel or otherwise. A repeat of a cancel that ended the order answers it
// too. Treat it as "already done" and read the order for how it ended.
func IsOrderAlreadyTerminal(err error) bool {
	return err != nil && apiErrorCode(err) == orderv1.ErrorCode_ERROR_CODE_ORDER_ALREADY_TERMINAL.String()
}

func apiErrorCode(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Code
	}
	return ""
}
