package transport

import (
	"errors"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// A REST 403 that names a service refusal is an AuthError with that code and
// the problem's detail. An order refusal's code is spelled as its ConnectRPC
// detail spells it (ERROR_CODE_SUBACCOUNT_READ_FORBIDDEN), so both transports
// return the same code; the problem keeps the code as sent.
func TestPoly8757RESTServiceRefusalKeepsItsCode(t *testing.T) {
	for _, code := range []string{"SUBACCOUNT_READ_FORBIDDEN", "QUICKSWAP_NOT_PERMITTED"} {
		err := callMeAgainst(t, http.StatusForbidden, "application/problem+json",
			`{"status":403,"code":"`+code+`","detail":"not allowed"}`)
		var auth *sdkerrors.AuthError
		if !errors.As(err, &auth) {
			t.Fatalf("%s: got %T %v", code, err, err)
		}
		if auth.Code != "ERROR_CODE_"+code || auth.Status != http.StatusForbidden || auth.Msg != "not allowed" {
			t.Fatalf("%s: got %+v", code, auth)
		}
		if auth.Problem == nil || auth.Problem.Code != code {
			t.Fatalf("%s: problem %+v", code, auth.Problem)
		}
		if got := sdkerrors.AuthErrorCode(err); got != "" {
			t.Fatalf("%s: auth code %q, want none", code, got)
		}
		if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("%s: retryable or unknown outcome", code)
		}
	}
	// Control: a credential refusal keeps its auth reason.
	err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"status":401,"code":"IP_NOT_ALLOWED","detail":"ip"}`)
	var auth *sdkerrors.AuthError
	if !errors.As(err, &auth) || sdkerrors.AuthErrorCode(err) != sdkerrors.AuthCodeIPNotAllowed {
		t.Fatalf("IP_NOT_ALLOWED: got %T %+v", err, err)
	}
}

// POLY-8756: the documented HTTP 400 MISSING_HEADERS problem keeps its auth
// reason, as the HTTP 401 one does.
func TestPoly8756MissingHeadersKeepsItsAuthReasonAt400(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		err := callMeAgainst(t, status, "application/problem+json",
			`{"code":"MISSING_HEADERS","detail":"missing signature headers"}`)
		if got := sdkerrors.AuthErrorCode(err); got != sdkerrors.AuthCodeMissingHeaders {
			t.Fatalf("%d: auth code %q from %T %v", status, got, err, err)
		}
	}
}
