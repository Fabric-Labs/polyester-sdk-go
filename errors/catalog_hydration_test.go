package errors

import (
	"errors"
	"fmt"
	"testing"
)

// A call that needs the catalogs fails before it sends anything when the
// client's construction-time catalog load failed, and the client never loads
// them again. Whatever that load failed with, the call is not retryable on the
// same client and its mutation outcome is known: nothing was sent.
func TestCatalogHydrationErrorIsNeverRetryableOrOutcomeUnknown(t *testing.T) {
	retryAfter := 1.5
	causes := []error{
		&TransportError{Msg: "dial tcp: connection refused"},
		&ServerError{Msg: "bad gateway", Status: 502},
		&ServerError{Msg: "unavailable", Status: 503, Code: "ERROR_CODE_FEE_SCHEDULE_UNAVAILABLE", AdmissionRefused: true},
		&RateLimitError{Msg: "rate limited", Code: "RATE_LIMIT_EXCEEDED", RetryAfter: &retryAfter},
		&RateLimitError{Msg: "replay record full", Code: AuthCodeReplayCapacityExhausted},
		&TimestampSkewError{Msg: "timestamp skew"},
		&APIError{Msg: "account repairing", Code: "ERROR_CODE_ACCOUNT_REPAIRING"},
		&APIError{Msg: "temporarily unavailable", Code: AuthCodeTemporarilyUnavailable},
		&APIError{Msg: "deadline exceeded", Code: CodeRequestDeadlineExceeded},
		&APIError{Msg: "outcome unknown", Code: "ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN"},
		&ResponseContractError{Operation: "GetSpotConfig", Msg: "missing field"},
		&RouteNotFoundError{Procedure: "/marketdata.v1.MarketDataService/GetSpotConfig"},
	}
	for _, cause := range causes {
		// The client wraps the load's failure before it records it.
		hydration := &CatalogHydrationError{Err: fmt.Errorf("catalog spot hydrate: %w", cause)}
		for name, err := range map[string]error{
			"bare":    hydration,
			"wrapped": fmt.Errorf("orderbook: %w", hydration),
		} {
			if IsRetryable(err) {
				t.Errorf("IsRetryable(%s hydration error over %T %q) = true; the client never loads its catalogs again", name, cause, cause)
			}
			if MutationOutcomeUnknown(err) {
				t.Errorf("MutationOutcomeUnknown(%s hydration error over %T %q) = true; nothing was sent", name, cause, cause)
			}
			// The cause stays reachable for diagnostics.
			if !errors.Is(err, cause) || !errors.Is(err, ErrPolyester) {
				t.Errorf("errors.Is(%s hydration error, %T) = false", name, cause)
			}
			target := &CatalogHydrationError{}
			if !errors.As(err, &target) || target != hydration {
				t.Errorf("errors.As(%s hydration error) did not find the *CatalogHydrationError", name)
			}
		}
	}

	// The same causes keep their own classification without the wrapper.
	if !MutationOutcomeUnknown(&TransportError{Msg: "reset"}) || IsRetryable(&TransportError{Msg: "reset"}) {
		t.Fatal("a bare transport failure is an unknown outcome")
	}
	if !IsRetryable(&RateLimitError{Msg: "rate limited"}) || MutationOutcomeUnknown(&RateLimitError{Msg: "rate limited"}) {
		t.Fatal("a bare rate-limit refusal is retryable with a known outcome")
	}
}
