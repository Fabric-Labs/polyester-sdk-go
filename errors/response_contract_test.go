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

func TestOrderOutcomeUnknownCodesRequireReconciliation(t *testing.T) {
	for _, code := range []string{"ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN", "ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN"} {
		err := &APIError{Msg: "not confirmed in time", Code: code}
		if !MutationOutcomeUnknown(err) {
			t.Fatalf("%s must require reading the order back", code)
		}
		if !IsRetryable(err) {
			t.Fatalf("%s may be retried after reconciliation", code)
		}
	}
	repairing := &APIError{Msg: "try again shortly", Code: "ERROR_CODE_ACCOUNT_REPAIRING"}
	if !IsRetryable(repairing) || MutationOutcomeUnknown(repairing) {
		t.Fatal("ERROR_CODE_ACCOUNT_REPAIRING changed nothing and is retryable")
	}
	orderRepairing := &APIError{Msg: "order being canceled", Code: "ERROR_CODE_ORDER_REPAIRING"}
	if IsRetryable(orderRepairing) || MutationOutcomeUnknown(orderRepairing) {
		t.Fatal("ERROR_CODE_ORDER_REPAIRING changed nothing, and the order is being canceled")
	}
	if MutationOutcomeUnknown(&APIError{Code: "ERROR_CODE_BAD_PRICE"}) || IsRetryable(&APIError{Code: "ERROR_CODE_BAD_PRICE"}) {
		t.Fatal("ordinary refusals stay final")
	}
}
