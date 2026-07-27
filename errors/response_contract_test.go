package errors

import "testing"

func TestResponseContractIsOutcomeUnknownButNotRetryable(t *testing.T) {
	err := &ResponseContractError{Operation: "CreateOrder", Msg: "missing order_id"}
	if IsRetryable(err) {
		t.Fatal("response contract errors must not be blindly retried")
	}
	if !MutationOutcomeUnknown(err) {
		t.Fatal("response contract errors must require reconciliation")
	}
}
