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
	// AuthCodeWalletAlreadyLinked: a valid signed wallet login named an
	// account address no account uses, and the signing wallet is already
	// linked to another account (Connect already_exists, REST 409). Wallets
	// are not moved between accounts and nothing changed. It is terminal:
	// the same request answers the same refusal. Log in with the account
	// address the wallet already uses.
	AuthCodeWalletAlreadyLinked = "AUTH_WALLET_ALREADY_LINKED"
)

// CodeRequestTooLarge is the code of an *APIError for a request body over the
// endpoint's size limit (REST problem REQUEST_TOO_LARGE, HTTP 413). The API
// refuses it before reading or authenticating the request, so nothing
// changed, and sending the same body again is refused again: reduce it to fit
// the limit. Read it with IsRequestTooLarge.
const CodeRequestTooLarge = "REQUEST_TOO_LARGE"

// IsRequestTooLarge reports whether err refuses a request body over the
// endpoint's size limit (REQUEST_TOO_LARGE).
func IsRequestTooLarge(err error) bool {
	return err != nil && apiErrorCode(err) == CodeRequestTooLarge
}

// IsWalletAlreadyLinked reports whether err refuses a wallet login because the
// wallet is already linked to another account (AUTH_WALLET_ALREADY_LINKED).
func IsWalletAlreadyLinked(err error) bool {
	return AuthErrorCode(err) == AuthCodeWalletAlreadyLinked
}

// API-key refusals (auth.v1.AuthErrorCode). Order reads, order and trigger
// writes and balance reads answer them on Connect as unauthenticated with an
// auth.v1.AuthErrorDetail; REST problem bodies name them without the AUTH_
// prefix. Only AuthCodeTimestampSkew is retryable, as TimestampSkewError; the
// others need a change by the caller. AuthCodeReplayCapacityExhausted is not a
// credentials refusal: it is a retryable *RateLimitError.
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
	// AuthCodeRecvWindowInvalid: the X-API-RECV-WINDOW header is not one
	// whole number of milliseconds from 1 to 60000, is empty, or is repeated
	// (REST RECV_WINDOW_INVALID, HTTP 401; Connect unauthenticated). The
	// request was not processed, and the same header is refused again: fix
	// Config.RecvWindow. Read it with IsRecvWindowInvalid.
	AuthCodeRecvWindowInvalid = "AUTH_RECV_WINDOW_INVALID"
)

// AuthCodeReplayCapacityExhausted refuses a signed API-key request because
// the key, or its account, holds its full share of the server's replay record:
// every signed request is remembered until its receive window ends (REST
// REPLAY_CAPACITY_EXHAUSTED, HTTP 429; Connect resource_exhausted with an
// auth.v1.AuthErrorDetail). It is a *RateLimitError with this Code, not an
// authentication failure: the credentials are valid, the request was not
// processed, and the share frees as earlier requests' windows end.
// IsRetryable reports it. It carries no Retry-After, so
// RetryAfter is nil unless the response named one: back off briefly, then
// send again, which signs again with a fresh timestamp. A shorter
// Config.RecvWindow makes room sooner. Read it with IsReplayCapacityExhausted.
const AuthCodeReplayCapacityExhausted = "AUTH_REPLAY_CAPACITY_EXHAUSTED"

// IsRecvWindowInvalid reports whether err refuses the request's
// X-API-RECV-WINDOW header (AUTH_RECV_WINDOW_INVALID).
func IsRecvWindowInvalid(err error) bool {
	return AuthErrorCode(err) == AuthCodeRecvWindowInvalid
}

// IsReplayCapacityExhausted reports whether err refuses a signed request
// because the key's or account's share of the replay record is full
// (AUTH_REPLAY_CAPACITY_EXHAUSTED). It is retryable after a short backoff.
func IsReplayCapacityExhausted(err error) bool {
	return AuthErrorCode(err) == AuthCodeReplayCapacityExhausted
}

// CodeRequestDeadlineExceeded is the code of an *APIError for an order or
// trigger write whose receive window ended before it was sent on for
// admission (ERROR_CODE_REQUEST_DEADLINE_EXCEEDED, 308; REST 400
// REQUEST_DEADLINE_EXCEEDED, Connect failed_precondition). It passed
// authentication, but nothing changed and it is never an unknown outcome:
// IsRetryable reports it, and sending it again signs it with a fresh
// timestamp. Read it with IsRequestDeadlineExceeded.
const CodeRequestDeadlineExceeded = "ERROR_CODE_REQUEST_DEADLINE_EXCEEDED"

// IsRequestDeadlineExceeded reports whether err refuses a write whose receive
// window ended before admission (ERROR_CODE_REQUEST_DEADLINE_EXCEEDED).
func IsRequestDeadlineExceeded(err error) bool {
	return err != nil && apiErrorCode(err) == CodeRequestDeadlineExceeded
}

// AuthErrorCode returns the auth.v1.AuthErrorCode name of err, such as
// "AUTH_SIGNATURE_INVALID", or "" when err carries no auth reason. It reads
// the Connect auth.v1.AuthErrorDetail code and the REST problem code alike: a
// REST code gains the AUTH_ prefix ("IP_NOT_ALLOWED" reads
// "AUTH_IP_NOT_ALLOWED"), and REST "UNAUTHENTICATED" reads
// "AUTH_AUTHENTICATION_REQUIRED". A TimestampSkewError reads
// "AUTH_TIMESTAMP_SKEW", a SessionRequiredError
// "AUTH_SESSION_KIND_NOT_ALLOWED", and a RateLimitError its auth code, such as
// "AUTH_REPLAY_CAPACITY_EXHAUSTED". Compare with the AuthCode* constants.
func AuthErrorCode(err error) string {
	var skewErr *TimestampSkewError
	if errors.As(err, &skewErr) {
		return AuthCodeTimestampSkew
	}
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) && strings.HasPrefix(rateErr.Code, "AUTH_") {
		return rateErr.Code
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
//
// Err is the caller's context error when the call's context was cancelled or
// its deadline passed, so errors.Is(err, context.Canceled) and
// errors.Is(err, context.DeadlineExceeded) report it. It is nil otherwise.
type TransportError struct {
	Msg string
	Err error
}

func (e *TransportError) Error() string        { return e.Msg }
func (e *TransportError) Is(target error) bool { return target == ErrPolyester }
func (e *TransportError) Unwrap() error        { return e.Err }

// RedirectRefusedError is returned when a request signed with an API key is
// answered with an HTTP redirect. The SDK never follows such a redirect: the
// signature, key ID and timestamp would reach the redirect target, which could
// replay the request within its receive window, and the signature covers the
// path, so even a same-origin redirect to another path could not verify.
//
// Nothing was sent to the redirect target. A redirect does not prove that the
// configured API URL did not apply the request before answering with it, so
// MutationOutcomeUnknown reports true: read a mutation back before sending it
// again. Sending it again gets the same redirect, so IsRetryable reports false.
// Point Config.APIURL at the API's own origin instead.
//
// Any request, signed or not, whose redirect names a Location that does not
// parse as a URL gets one too, with Malformed set: such a redirect cannot be
// followed, and the error never repeats the Location, which may carry
// credentials in its userinfo or query.
type RedirectRefusedError struct {
	// StatusCode is the redirect status, such as 307 or 308.
	StatusCode int
	// Origin is the scheme, host and port the redirect named, without its
	// path, query or userinfo.
	Origin string
	// SameOrigin reports whether Origin is the origin the request was sent to.
	SameOrigin bool
	// Malformed reports that the redirect's Location did not parse as a URL.
	// Origin is then empty and SameOrigin false.
	Malformed bool
}

func (e *RedirectRefusedError) Error() string {
	if e.Malformed {
		return "request was redirected (HTTP " + strconv.Itoa(e.StatusCode) +
			") to a malformed Location; the SDK refuses it without following: set the API URL to the API's origin"
	}
	where := "to another origin, " + e.Origin
	if e.SameOrigin {
		where = "within its origin, " + e.Origin
	}
	return "signed request was redirected (HTTP " + strconv.Itoa(e.StatusCode) + ") " + where +
		"; the SDK does not follow redirects of signed requests: set the API URL to the API's origin"
}
func (e *RedirectRefusedError) Is(target error) bool { return target == ErrPolyester }

// CatalogHydrationError reports a catalog the client loads when it is created
// that could not be loaded. The client loads two, each on its own: the spot
// config, which every call that names a market or needs a pair's scales
// waits for, and the funding (deposit/withdraw) config, which only
// Zipper.SubscribeZippedAssetSupply waits for. Those calls return this error
// instead of refusing the symbol, as do Client.WaitForCatalogs (spot config)
// and Client.WaitForFundingCatalog (funding config). Err is the last failure,
// so errors.Is and errors.As reach it. Create a new client to load the
// catalogs again.
//
// Catalog is "spot" or "funding", Operation the Connect procedure, such as
// chain.zipper.v1.ZipperService/GetDepositWithdrawConfig, Failure one of
// "timeout", "connection_closed", "connect_failed", "unavailable", "invalid"
// (the response did not validate) or "error", Attempts how many times it was
// read, and ResponseRef the edge's reference of the last failed response
// (its cf-ray header), when one arrived. They are empty on an error that
// does not come from the client's own load. The message carries no
// credentials and no proxy URL.
//
// The call sent nothing, and the client never loads its catalogs again, so
// IsRetryable and MutationOutcomeUnknown report false whatever Err is, even a
// *TransportError, a *ServerError or a *RateLimitError: calling again on the
// same client fails the same way. Predicates that name a cause, such as
// AuthErrorCode and IsReplayCapacityExhausted, still read Err for diagnostics.
type CatalogHydrationError struct {
	Err         error
	Catalog     string
	Operation   string
	Failure     string
	Attempts    int
	ResponseRef string
	// Msg, when set, is the message after "catalog hydration failed: ",
	// already redacted; otherwise it is Err's text.
	Msg string
}

func (e *CatalogHydrationError) Error() string {
	if e.Msg != "" {
		return "catalog hydration failed: " + e.Msg
	}
	return "catalog hydration failed: " + e.Err.Error()
}
func (e *CatalogHydrationError) Is(target error) bool { return target == ErrPolyester }
func (e *CatalogHydrationError) Unwrap() error        { return e.Err }

// RateLimitError indicates rate limiting from the API.
//
// Detail carries structured polyester.ratelimit.v1.RateLimitDetail when the
// server attached it: Connect details, an in-band order rejection, or the
// rate_limit object of a REST RATE_LIMIT_EXCEEDED problem, whose fields are
// all optional. The gateway's own refusal names only Reason and Scope
// (CLIENT_IP, with no OperationID); a service's also names the operation, the
// policy class and the remaining quota. RetryAfter prefers
// detail.retry_after_ms, then the Retry-After header; it is nil when the
// server gave no hint, as for a refusal of too many concurrent requests, so
// back off on your own schedule then.
type RateLimitError struct {
	Code       string // Structured service code; empty without a service detail. Known order codes use ERROR_CODE_; an orders detail without a code uses ERROR_CODE_UNSPECIFIED.
	Msg        string
	RetryAfter *float64
	Detail     *RateLimitDetail
}

func (e *RateLimitError) Error() string        { return e.Msg }
func (e *RateLimitError) Is(target error) bool { return target == ErrPolyester }

// ServerError indicates a server-side failure, or any HTTP error that can't
// prove a refusal: a 5xx, or a 4xx other than 401 and 403 with no code the
// contract defines. A gateway or an edge can send those too.
type ServerError struct {
	// AdmissionRefused is true only for a typed orders-service refusal.
	AdmissionRefused bool
	Msg              string
	Code             string // Service code, or an unrecognised code kept for diagnosis.
	Status           int    // HTTP status corresponding to the failure, when known.
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
// requested operation (Connect unimplemented, or a contract code at HTTP 501).
// Retrying does not help.
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

// RouteNotFoundError indicates the gateway has no route for an RPC: a bare HTTP
// 404. Its mutation outcome is unknown, because whatever answered isn't the
// service and can't prove that a mutation wasn't applied.
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

// SubscriptionEndReason says why a subscription the server ended did not resume.
type SubscriptionEndReason string

const (
	// SubscriptionEndNotResubscribable means the channel is not one a
	// sub-account's access governs, or automatic reconnection is off. Subscribe
	// again yourself if the channel should still be live.
	SubscriptionEndNotResubscribable SubscriptionEndReason = "not_resubscribable"
	// SubscriptionEndAccessRemoved means a sub-account update on this client said
	// the caller no longer has access (removed true).
	SubscriptionEndAccessRemoved SubscriptionEndReason = "access_removed"
	// SubscriptionEndNotListed means ListSubaccounts no longer lists the
	// sub-account with a role for this API key.
	SubscriptionEndNotListed SubscriptionEndReason = "not_listed"
	// SubscriptionEndListRefused means ListSubaccounts refused this API key, so
	// access could not be confirmed. Cause holds the refusal.
	SubscriptionEndListRefused SubscriptionEndReason = "list_refused"
	// SubscriptionEndTokenRefused means the server refused a fresh subscribe
	// token. Cause holds the refusal.
	SubscriptionEndTokenRefused SubscriptionEndReason = "token_refused"
	// SubscriptionEndRetriesExhausted means bounded attempts to subscribe again
	// did not hold. Cause holds the last failure, if any.
	SubscriptionEndRetriesExhausted SubscriptionEndReason = "retries_exhausted"
)

// SubscriptionEndedError reports that the server ended a realtime subscription
// and the SDK stopped. Code and ServerReason are the server's unsubscribe code
// and text; Reason says why it did not resume. Queued messages stay readable.
type SubscriptionEndedError struct {
	Channel      string
	Code         uint32
	ServerReason string
	Reason       SubscriptionEndReason
	Cause        error
}

func (e *SubscriptionEndedError) Error() string {
	msg := "server ended realtime subscription to " + strconv.Quote(e.Channel) +
		" (code " + strconv.FormatUint(uint64(e.Code), 10) + ": " + e.ServerReason + "): " + string(e.Reason)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *SubscriptionEndedError) Unwrap() error { return e.Cause }

func (e *SubscriptionEndedError) Is(target error) bool { return target == ErrPolyester }

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

// IsRetryable reports whether retrying may succeed with a known outcome.
// Unknown mutation outcomes always return false, including bare server and
// transport failures. Read back before sending a mutation again; safe reads
// may retry explicitly. A client order ID enables lookup, not exact replay.
// Explicit admission refusals, rate refusals (including replay capacity),
// timestamp skew, a request deadline exceeded before admission and account
// repairing may retry. A *CatalogHydrationError is never retryable, whatever
// the catalog load failed with: create a new client.
func IsRetryable(err error) bool {
	if isCatalogHydrationFailure(err) {
		return false
	}
	var transportErr *TransportError
	var rateLimitErr *RateLimitError
	var serverErr *ServerError
	var skewErr *TimestampSkewError
	code := apiErrorCode(err)
	if MutationOutcomeUnknown(err) {
		return false
	}
	return errors.As(err, &transportErr) ||
		errors.As(err, &rateLimitErr) ||
		errors.As(err, &serverErr) ||
		errors.As(err, &skewErr) ||
		code == "ERROR_CODE_ACCOUNT_REPAIRING" ||
		code == CodeRequestDeadlineExceeded ||
		code == AuthCodeTemporarilyUnavailable
}

// MutationOutcomeUnknown reports whether the server may have applied a failed
// mutation before the error became visible to the caller. It includes every
// order error code ending in _OUTCOME_UNKNOWN: an order create, cancel or
// modify, or a trigger create, cancel, pause, resume or modify, that was sent
// but not confirmed in time. It also includes any HTTP error with no recognised
// code except a 401 or 403, and a bare 404, and a *RedirectRefusedError: the
// origin that answered with the redirect may have applied the request. Read
// the order or trigger back before retrying. A *CatalogHydrationError never
// has an unknown outcome, whatever the catalog load failed with: the call
// failed before it sent anything.
func MutationOutcomeUnknown(err error) bool {
	if isCatalogHydrationFailure(err) {
		return false
	}
	var transportErr *TransportError
	var serverErr *ServerError
	var contractErr *ResponseContractError
	var routeErr *RouteNotFoundError
	var redirectErr *RedirectRefusedError
	serverUnknown := errors.As(err, &serverErr)
	if serverUnknown && serverErr.Status == 503 && (serverErr.AdmissionRefused || isKnownAdmissionRefusal(serverErr.Code)) {
		serverUnknown = false
	}
	return errors.As(err, &transportErr) || serverUnknown ||
		errors.As(err, &contractErr) || errors.As(err, &routeErr) || errors.As(err, &redirectErr) ||
		isOutcomeUnknownCode(apiErrorCode(err))
}

// isCatalogHydrationFailure reports whether err is or wraps a
// *CatalogHydrationError. The classifiers check it before they look through it
// at the catalog load's failure, which says nothing about the call.
func isCatalogHydrationFailure(err error) bool {
	var hydrationErr *CatalogHydrationError
	return errors.As(err, &hydrationErr)
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

// isKnownAdmissionRefusal reports an explicit orders-service admission refusal.
// A bare status or an unrecognized service code does not establish this.
func isKnownAdmissionRefusal(code string) bool {
	switch strings.TrimPrefix(code, "ERROR_CODE_") {
	case "QUICKSWAP_ACCOUNT_OWNER_UNKNOWN", "FEE_SCHEDULE_UNAVAILABLE", "POLICY_UNAVAILABLE", "RATE_LIMIT_POLICY_UNAVAILABLE", "PAIR_BACKLOGGED", "SETTLEMENT_BACKLOGGED", "ORDER_MODIFY_IN_PROGRESS", "ORDER_PLACEMENT_PENDING", "TRIGGER_CONTROLS_BUSY", "TRIGGER_CONTROL_IN_PROGRESS":
		return true
	default:
		return false
	}
}
