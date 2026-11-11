package errors

import "errors"

// Price-alert refusals (alerts.v1.ErrorCode), as the Code of the error the
// Alerts service returns. Read one with AlertErrorCode.
const (
	// AlertCodeNotFound: no such alert. Another account's alert reads the same.
	AlertCodeNotFound = "ALERT_NOT_FOUND"
	// AlertCodeUnknownSymbol: the market does not exist.
	AlertCodeUnknownSymbol = "UNKNOWN_SYMBOL"
	// AlertCodePriceUnavailable: the market has no current price to record
	// as the reference; nothing changed. A retryable *ServerError.
	AlertCodePriceUnavailable = "PRICE_UNAVAILABLE"
	// AlertCodePriceSourceUnsupported: the market has no such price, such as
	// an index price.
	AlertCodePriceSourceUnsupported = "PRICE_SOURCE_UNSUPPORTED"
	// AlertCodeLimitReached: the account holds as many alerts as it may.
	AlertCodeLimitReached = "ALERT_LIMIT_REACHED"
	// AlertCodeRevisionConflict: ExpectedRevision is stale. Read the alert
	// again.
	AlertCodeRevisionConflict = "REVISION_CONFLICT"
	// AlertCodeCancelled: the alert is cancelled and cannot change.
	AlertCodeCancelled = "ALERT_CANCELLED"
	// AlertCodeTriggered: the alert already fired.
	AlertCodeTriggered = "ALERT_TRIGGERED"
	// AlertCodeIdempotencyConflict: the idempotency key was already used for
	// another request (Connect already_exists).
	AlertCodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	// AlertCodeServiceUnavailable: the alert service cannot answer now. A
	// *ServerError whose outcome is unknown, as for other services: after a
	// while, send a read again, send Create or Update again with the same
	// idempotency key, or Cancel again (it is idempotent).
	AlertCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	// AlertCodeSubaccountKeyRefused: alerts belong to the main account; an API
	// key scoped to a sub-account is refused (permission_denied). An
	// *AuthError.
	AlertCodeSubaccountKeyRefused = "SUBACCOUNT_KEY_REFUSED"
	// AlertCodeUpdateLimitReached: the alert holds its 1000 update
	// idempotency keys; repeats of stored keys and updates without a key still
	// work.
	AlertCodeUpdateLimitReached = "ALERT_UPDATE_LIMIT_REACHED"
)

var alertCodes = map[string]bool{
	AlertCodeNotFound: true, AlertCodeUnknownSymbol: true, AlertCodePriceUnavailable: true,
	AlertCodePriceSourceUnsupported: true, AlertCodeLimitReached: true, AlertCodeRevisionConflict: true,
	AlertCodeCancelled: true, AlertCodeTriggered: true, AlertCodeIdempotencyConflict: true,
	AlertCodeServiceUnavailable: true, AlertCodeSubaccountKeyRefused: true, AlertCodeUpdateLimitReached: true,
}

// AlertErrorCode returns the alerts.v1.ErrorCode name of an error the Alerts
// service returned, such as AlertCodeRevisionConflict, or "" for any other
// error. The refusal is a *APIError, a *ServerError
// (AlertCodePriceUnavailable, AlertCodeServiceUnavailable) or an *AuthError
// (AlertCodeSubaccountKeyRefused) whose Code is that name.
func AlertErrorCode(err error) string {
	var apiErr *APIError
	var serverErr *ServerError
	var authErr *AuthError
	code := ""
	switch {
	case errors.As(err, &apiErr):
		code = apiErr.Code
	case errors.As(err, &serverErr):
		code = serverErr.Code
	case errors.As(err, &authErr):
		code = authErr.Code
	}
	if alertCodes[code] {
		return code
	}
	return ""
}
