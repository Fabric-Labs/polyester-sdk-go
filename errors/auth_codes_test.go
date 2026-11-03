package errors_test

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestAuthErrorCodeReadsEveryTransport(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&sdkerrors.APIError{Code: "AUTH_REVISION_CONFLICT"}, "AUTH_REVISION_CONFLICT"},
		{&sdkerrors.AuthError{Code: "AUTH_KEY_EXPIRED"}, sdkerrors.AuthCodeKeyExpired},
		{&sdkerrors.AuthError{Code: "KEY_DISABLED"}, sdkerrors.AuthCodeKeyDisabled},
		{&sdkerrors.AuthError{Code: "MISSING_HEADERS"}, sdkerrors.AuthCodeMissingHeaders},
		{&sdkerrors.AuthError{Code: "API_KEY_NOT_ACCEPTED"}, sdkerrors.AuthCodeAPIKeyNotAccepted},
		{&sdkerrors.AuthError{Code: "UNAUTHENTICATED"}, sdkerrors.AuthCodeAuthenticationRequired},
		{&sdkerrors.TimestampSkewError{Msg: "skew", Status: 401}, sdkerrors.AuthCodeTimestampSkew},
		{&sdkerrors.SessionRequiredError{}, sdkerrors.AuthCodeSessionKindNotAllowed},
		// Not auth reasons: Connect codes, order codes, unknown REST codes.
		{&sdkerrors.AuthError{Code: "unauthenticated"}, ""},
		{&sdkerrors.AuthError{Code: "ERROR_CODE_UNAUTHENTICATED"}, ""},
		{&sdkerrors.APIError{Code: "ERROR_CODE_ORDER_UNKNOWN"}, ""},
		{&sdkerrors.AuthError{Code: "INVALID_SIGNATURE"}, ""},
		{errors.New("AUTH_KEY_EXPIRED"), ""},
	} {
		if got := sdkerrors.AuthErrorCode(tc.err); got != tc.want {
			t.Errorf("AuthErrorCode(%#v)=%q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestSessionRequiredIsFinal(t *testing.T) {
	err := &sdkerrors.SessionRequiredError{Msg: "interactive session required", Status: 403}
	if sdkerrors.IsRetryable(err) || !sdkerrors.IsSessionRequired(err) || err.Error() != "interactive session required" {
		t.Fatalf("err=%v", err)
	}
	if (&sdkerrors.SessionRequiredError{}).Error() == "" {
		t.Fatal("empty message")
	}
}

func TestIsOrderAlreadyTerminal(t *testing.T) {
	if !sdkerrors.IsOrderAlreadyTerminal(&sdkerrors.APIError{Code: "ERROR_CODE_ORDER_ALREADY_TERMINAL"}) {
		t.Fatal("ORDER_ALREADY_TERMINAL is already ended")
	}
	if !sdkerrors.IsOrderAlreadyTerminal(&sdkerrors.ValidationError{Code: "ERROR_CODE_ORDER_ALREADY_TERMINAL"}) {
		t.Fatal("ORDER_ALREADY_TERMINAL with violations is already ended")
	}
	if sdkerrors.IsOrderAlreadyTerminal(&sdkerrors.APIError{Code: "ERROR_CODE_ORDER_UNKNOWN"}) || sdkerrors.IsOrderAlreadyTerminal(nil) {
		t.Fatal("other codes are not already ended")
	}
	if sdkerrors.IsRetryable(&sdkerrors.APIError{Code: "ERROR_CODE_ORDER_ALREADY_TERMINAL"}) {
		t.Fatal("already ended is not retryable")
	}
}
