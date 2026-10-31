package errors

import (
	"strings"
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

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
	for _, code := range []string{
		"ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN",
		"ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN",
		"ERROR_CODE_ORDER_MODIFY_OUTCOME_UNKNOWN",
		"ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN",
		"ERROR_CODE_TRIGGER_CONTROL_OUTCOME_UNKNOWN",
	} {
		assertOutcomeUnknown(t, code)
	}
	repairing := &APIError{Msg: "try again shortly", Code: "ERROR_CODE_ACCOUNT_REPAIRING"}
	if !IsRetryable(repairing) || MutationOutcomeUnknown(repairing) {
		t.Fatal("ERROR_CODE_ACCOUNT_REPAIRING changed nothing and is retryable")
	}
	authUnavailable := &APIError{Msg: "try again", Code: AuthCodeTemporarilyUnavailable}
	if !IsRetryable(authUnavailable) || MutationOutcomeUnknown(authUnavailable) {
		t.Fatal("AUTH_TEMPORARILY_UNAVAILABLE changed nothing and is retryable")
	}
	orderRepairing := &APIError{Msg: "order being canceled", Code: "ERROR_CODE_ORDER_REPAIRING"}
	if IsRetryable(orderRepairing) || MutationOutcomeUnknown(orderRepairing) {
		t.Fatal("ERROR_CODE_ORDER_REPAIRING changed nothing, and the order is being canceled")
	}
	if MutationOutcomeUnknown(&APIError{Code: "ERROR_CODE_BAD_PRICE"}) || IsRetryable(&APIError{Code: "ERROR_CODE_BAD_PRICE"}) {
		t.Fatal("ordinary refusals stay final")
	}
}

// Every generated order error code ending in _OUTCOME_UNKNOWN is classified,
// so a code added to orders.v1 is covered without a hand-maintained list.
func TestEveryGeneratedOutcomeUnknownCodeRequiresReconciliation(t *testing.T) {
	found := 0
	for _, code := range orderv1.ErrorCode_name {
		if !strings.HasSuffix(code, "_OUTCOME_UNKNOWN") {
			continue
		}
		found++
		assertOutcomeUnknown(t, code)
	}
	if found == 0 {
		t.Fatal("orders.v1 ErrorCode has no _OUTCOME_UNKNOWN values")
	}
}

func assertOutcomeUnknown(t *testing.T, code string) {
	t.Helper()
	err := &APIError{Msg: "not confirmed in time", Code: code}
	if !MutationOutcomeUnknown(err) {
		t.Errorf("%s must require reading the order or trigger back", code)
	}
	if IsRetryable(err) {
		t.Errorf("%s must not be retried before reconciliation", code)
	}
}
