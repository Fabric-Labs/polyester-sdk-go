package transport

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// Every REST endpoint answers a credential refusal with the bare documented
// code and status, including the account, daily-claim and rewards endpoints
// that used AUTH_AUTHENTICATION_REQUIRED or AUTH_-prefixed codes. Each reads
// as the same auth reason its ConnectRPC detail carries.
func TestRESTCredentialRefusalsReadTheirAuthReason(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   string
	}{
		{http.StatusUnauthorized, "UNAUTHENTICATED", sdkerrors.AuthCodeAuthenticationRequired},
		{http.StatusUnauthorized, "MISSING_HEADERS", sdkerrors.AuthCodeMissingHeaders},
		{http.StatusUnauthorized, "SIGNATURE_INVALID", sdkerrors.AuthCodeSignatureInvalid},
		{http.StatusUnauthorized, "RECV_WINDOW_INVALID", sdkerrors.AuthCodeRecvWindowInvalid},
		{http.StatusUnauthorized, "KEY_EXPIRED", sdkerrors.AuthCodeKeyExpired},
		{http.StatusUnauthorized, "KEY_DISABLED", sdkerrors.AuthCodeKeyDisabled},
		{http.StatusUnauthorized, "IP_NOT_ALLOWED", sdkerrors.AuthCodeIPNotAllowed},
	} {
		t.Run(tc.code, func(t *testing.T) {
			err := callMeAgainst(t, tc.status, "application/problem+json",
				fmt.Sprintf(`{"type":"https://polyester.dev/problems/x","title":"t","status":%d,"detail":"refused","code":%q}`, tc.status, tc.code))
			var auth *sdkerrors.AuthError
			if !errors.As(err, &auth) || auth.Status != tc.status || sdkerrors.AuthErrorCode(err) != tc.want {
				t.Fatalf("got %T %+v, auth code %q", err, err, sdkerrors.AuthErrorCode(err))
			}
			if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
				t.Fatalf("%s must be final with a known outcome", tc.code)
			}
		})
	}
	// A timestamp outside the receive window is retryable after a clock
	// resync, whatever the status.
	skew := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"status":401,"title":"t","detail":"API key timestamp is outside the request's receive window","code":"TIMESTAMP_SKEW"}`)
	if sdkerrors.AuthErrorCode(skew) != sdkerrors.AuthCodeTimestampSkew || !sdkerrors.IsRetryable(skew) || sdkerrors.MutationOutcomeUnknown(skew) {
		t.Fatalf("TIMESTAMP_SKEW: %T %+v", skew, skew)
	}
}

// Authentication that is temporarily unavailable changed nothing: REST 503
// AUTH_TEMPORARILY_UNAVAILABLE is a retryable refusal with a known outcome,
// exactly as the ConnectRPC detail reads, not a server error of unknown
// outcome.
func TestRESTAuthTemporarilyUnavailableIsARetryableRefusal(t *testing.T) {
	err := callMeAgainst(t, http.StatusServiceUnavailable, "application/problem+json",
		`{"status":503,"title":"Authentication Temporarily Unavailable","detail":"try again","code":"AUTH_TEMPORARILY_UNAVAILABLE"}`)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != sdkerrors.AuthCodeTemporarilyUnavailable {
		t.Fatalf("got %T %+v", err, err)
	}
	if sdkerrors.AuthErrorCode(err) != sdkerrors.AuthCodeTemporarilyUnavailable {
		t.Fatalf("auth code %q", sdkerrors.AuthErrorCode(err))
	}
	if !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
}

// Replay capacity is exhausted on rewards as everywhere else: a retryable 429
// rate refusal, never a server error.
func TestRESTReplayCapacityExhaustedIsARetryableRateRefusal(t *testing.T) {
	err := callMeAgainst(t, http.StatusTooManyRequests, "application/problem+json",
		`{"status":429,"title":"t","detail":"too many signed requests","code":"REPLAY_CAPACITY_EXHAUSTED"}`)
	var rate *sdkerrors.RateLimitError
	if !errors.As(err, &rate) || sdkerrors.AuthErrorCode(err) != sdkerrors.AuthCodeReplayCapacityExhausted {
		t.Fatalf("got %T %+v", err, err)
	}
	if !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
}

// A path the API does not serve answers a 404 problem with code NOT_FOUND: a
// final refusal with a known outcome, read as the ConnectRPC NOT_FOUND code.
func TestRESTNotFoundProblemKeepsItsCode(t *testing.T) {
	err := callMeAgainst(t, http.StatusNotFound, "application/problem+json",
		`{"type":"https://polyester.dev/problems/not-found","title":"Not Found","status":404,"detail":"No API operation matches this method and path.","code":"NOT_FOUND"}`)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != "ERROR_CODE_NOT_FOUND" || api.Problem == nil || api.Problem.Code != "NOT_FOUND" {
		t.Fatalf("got %T %+v", err, err)
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
}

// A trailing stop that has started tracking refuses a new nonzero activation
// price: REST 400 TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE reads as its
// ConnectRPC detail names it, a final refusal that changed nothing.
func TestRESTTriggerActivationPriceNotModifiableKeepsItsCode(t *testing.T) {
	err := callMeAgainst(t, http.StatusBadRequest, "application/problem+json",
		`{"status":400,"title":"Trigger Activation Price Not Modifiable","detail":"tracking started","code":"TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE"}`)
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != "ERROR_CODE_TRIGGER_ACTIVATION_PRICE_NOT_MODIFIABLE" {
		t.Fatalf("got %T %+v", err, err)
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
}
