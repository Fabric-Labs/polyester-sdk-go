package errors

import (
	"fmt"
	"testing"
)

func TestRequestTooLargeAndWalletAlreadyLinkedPredicates(t *testing.T) {
	tooLarge := &APIError{Msg: "request too large", Code: CodeRequestTooLarge}
	if !IsRequestTooLarge(tooLarge) || !IsRequestTooLarge(fmt.Errorf("wrapped: %w", tooLarge)) {
		t.Fatal("IsRequestTooLarge misses a REQUEST_TOO_LARGE refusal")
	}
	for _, err := range []error{nil, &APIError{Code: "PAYLOAD_TOO_LARGE"}, &RateLimitError{Code: "REQUEST_TOO_LARGE"}, &ServerError{Status: 413}} {
		if IsRequestTooLarge(err) {
			t.Fatalf("IsRequestTooLarge(%T %v) = true", err, err)
		}
	}
	if IsRetryable(tooLarge) || MutationOutcomeUnknown(tooLarge) {
		t.Fatal("REQUEST_TOO_LARGE is a terminal refusal")
	}

	linked := &APIError{Msg: "this wallet is already linked to another account", Code: AuthCodeWalletAlreadyLinked}
	if !IsWalletAlreadyLinked(linked) || AuthErrorCode(linked) != "AUTH_WALLET_ALREADY_LINKED" {
		t.Fatal("IsWalletAlreadyLinked misses AUTH_WALLET_ALREADY_LINKED")
	}
	// The REST problem code carries the AUTH_ prefix itself.
	if !IsWalletAlreadyLinked(&APIError{Code: "AUTH_WALLET_ALREADY_LINKED"}) || IsWalletAlreadyLinked(&APIError{Code: "AUTH_WALLET_LOGIN_FAILED"}) || IsWalletAlreadyLinked(nil) {
		t.Fatal("IsWalletAlreadyLinked reads the wrong codes")
	}
	if IsRetryable(linked) || MutationOutcomeUnknown(linked) {
		t.Fatal("AUTH_WALLET_ALREADY_LINKED is a terminal refusal")
	}
}
