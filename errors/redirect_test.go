package errors

import (
	"errors"
	"fmt"
	"testing"
)

// A refused redirect is never retryable, and its outcome is unknown: the
// origin that answered with the redirect may have applied the request.
func TestRedirectRefusedErrorClassification(t *testing.T) {
	refusal := &RedirectRefusedError{StatusCode: 307, Origin: "https://other.example"}
	for _, err := range []error{refusal, fmt.Errorf("wrapped: %w", refusal)} {
		if !errors.Is(err, ErrPolyester) {
			t.Fatalf("%v: not an SDK error", err)
		}
		if IsRetryable(err) {
			t.Fatalf("%v: retryable", err)
		}
		if !MutationOutcomeUnknown(err) {
			t.Fatalf("%v: known outcome", err)
		}
	}
}
