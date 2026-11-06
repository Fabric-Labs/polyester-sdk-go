package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
)

func TestMapConnectErrorSurfacesAuthRevisionConflict(t *testing.T) {
	mapped := mapAuthDetail(t, authv1.AuthErrorCode_AUTH_REVISION_CONFLICT, "resource changed")
	apiErr, ok := mapped.(*sdkerrors.APIError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if apiErr.Code != "AUTH_REVISION_CONFLICT" {
		t.Fatalf("code=%q", apiErr.Code)
	}
	if apiErr.Msg != "resource changed" {
		t.Fatalf("msg=%q", apiErr.Msg)
	}
}

func TestMapConnectErrorMapsCancellationAndValidationConcretely(t *testing.T) {
	var transportErr *sdkerrors.TransportError
	if err := MapConnectError(connect.NewError(connect.CodeCanceled, nil)); !errors.As(err, &transportErr) {
		t.Fatalf("canceled mapped to %T: %v", err, err)
	}
	var validationErr *sdkerrors.ValidationError
	if err := MapConnectError(connect.NewError(connect.CodeInvalidArgument, nil)); !errors.As(err, &validationErr) {
		t.Fatalf("invalid argument mapped to %T: %v", err, err)
	}
}

func TestMapConnectErrorPreservesOrderValidationDetails(t *testing.T) {
	connectErr := connect.NewError(connect.CodeInvalidArgument, nil)
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_PRICE_TICK_SIZE,
		Violations: []*orderv1.FieldViolation{{
			FieldPath: "order.limit_gtc.price_ticks",
			RuleId:    "price_tick_size",
			Message:   "price is off tick",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(mapped, &validationErr) {
		t.Fatalf("mapped=%T: %v", mapped, mapped)
	}
	if validationErr.Code != "ERROR_CODE_PRICE_TICK_SIZE" ||
		validationErr.Metadata["violation.0.rule_id"] != "price_tick_size" {
		t.Fatalf("validation detail=%#v", validationErr)
	}
}

func TestMapConnectErrorNeverReturnsEmptyAuthMessage(t *testing.T) {
	mapped := MapConnectError(connect.NewError(connect.CodeUnauthenticated, nil))
	authErr, ok := mapped.(*sdkerrors.AuthError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if authErr.Msg == "" {
		t.Fatal("expected non-empty auth error message")
	}
	if authErr.Code != "unauthenticated" || authErr.Status != 401 {
		t.Fatalf("unauthenticated code=%q status=%d", authErr.Code, authErr.Status)
	}
}

func TestMapConnectErrorStampsPermissionDeniedCode(t *testing.T) {
	mapped := MapConnectError(connect.NewError(connect.CodePermissionDenied, errors.New("address book write denied")))
	authErr, ok := mapped.(*sdkerrors.AuthError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if authErr.Msg != "address book write denied" {
		t.Fatalf("msg=%q", authErr.Msg)
	}
	if authErr.Code != "permission_denied" || authErr.Status != 403 {
		t.Fatalf("permission_denied code=%q status=%d", authErr.Code, authErr.Status)
	}
}

func TestMapConnectErrorSurfacesRetryAfter(t *testing.T) {
	connectErr := connect.NewError(connect.CodeResourceExhausted, nil)
	connectErr.Meta().Set("Retry-After", "2.5")
	mapped := MapConnectError(connectErr)
	rateLimitErr, ok := mapped.(*sdkerrors.RateLimitError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if rateLimitErr.RetryAfter == nil || *rateLimitErr.RetryAfter != 2.5 {
		t.Fatalf("retry_after=%v", rateLimitErr.RetryAfter)
	}
	if rateLimitErr.Detail != nil {
		t.Fatalf("expected nil detail, got %#v", rateLimitErr.Detail)
	}
	if !sdkerrors.IsRetryable(mapped) {
		t.Fatal("rate limit must be retryable")
	}
	if sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatal("rate-limit rejection must not be marked as ambiguous")
	}
}

func TestMapConnectErrorSurfacesNestedRateLimitDetail(t *testing.T) {
	limit := uint64(100)
	remaining := uint64(0)
	retryAfterMs := uint64(2500)
	policyVersion := uint64(3)
	connectErr := connect.NewError(connect.CodeResourceExhausted, nil)
	connectErr.Meta().Set("Retry-After", "9")
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED,
		RateLimit: &ratelimitv1.RateLimitDetail{
			Reason:        ratelimitv1.FailureReason_QUOTA_EXCEEDED,
			Limit:         &limit,
			Remaining:     &remaining,
			RetryAfterMs:  &retryAfterMs,
			PolicyVersion: &policyVersion,
			OperationId:   "orders.create",
			PolicyClass:   ratelimitv1.PolicyClass_TRADING_PLACE,
			Scope:         ratelimitv1.LimiterScope_API_KEY,
			RefillModel:   ratelimitv1.RefillModel_CONTINUOUS,
		},
	})
	if err != nil {
		t.Fatalf("NewErrorDetail: %v", err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	rateLimitErr, ok := mapped.(*sdkerrors.RateLimitError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if rateLimitErr.RetryAfter == nil || *rateLimitErr.RetryAfter != 2.5 {
		t.Fatalf("detail retry_after should win over header: %v", rateLimitErr.RetryAfter)
	}
	if rateLimitErr.Detail == nil {
		t.Fatal("expected rate limit detail")
	}
	if rateLimitErr.Detail.Reason != "QUOTA_EXCEEDED" ||
		rateLimitErr.Detail.OperationID != "orders.create" ||
		rateLimitErr.Detail.PolicyClass != "TRADING_PLACE" ||
		rateLimitErr.Detail.Scope != "API_KEY" ||
		rateLimitErr.Detail.RefillModel != "CONTINUOUS" ||
		rateLimitErr.Detail.Limit == nil || *rateLimitErr.Detail.Limit != 100 ||
		rateLimitErr.Detail.Remaining == nil || *rateLimitErr.Detail.Remaining != 0 ||
		rateLimitErr.Detail.RetryAfterMs == nil || *rateLimitErr.Detail.RetryAfterMs != 2500 ||
		rateLimitErr.Detail.PolicyVersion == nil || *rateLimitErr.Detail.PolicyVersion != 3 {
		t.Fatalf("detail=%#v", rateLimitErr.Detail)
	}
}

func TestMapConnectErrorSurfacesTopLevelRateLimitDetail(t *testing.T) {
	retryAfterMs := uint64(1250)
	connectErr := connect.NewError(connect.CodeResourceExhausted, nil)
	detail, err := connect.NewErrorDetail(&ratelimitv1.RateLimitDetail{
		Reason:       ratelimitv1.FailureReason_QUOTA_EXCEEDED,
		RetryAfterMs: &retryAfterMs,
		OperationId:  "auth.me",
		PolicyClass:  ratelimitv1.PolicyClass_AUTH_PUBLIC,
		Scope:        ratelimitv1.LimiterScope_CLIENT_IP,
	})
	if err != nil {
		t.Fatalf("NewErrorDetail: %v", err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	rateLimitErr, ok := mapped.(*sdkerrors.RateLimitError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if rateLimitErr.RetryAfter == nil || *rateLimitErr.RetryAfter != 1.25 {
		t.Fatalf("retry_after=%v", rateLimitErr.RetryAfter)
	}
	if rateLimitErr.Detail == nil || rateLimitErr.Detail.OperationID != "auth.me" {
		t.Fatalf("detail=%#v", rateLimitErr.Detail)
	}
}

func TestTransportRetryClassificationPreservesAmbiguity(t *testing.T) {
	err := &sdkerrors.TransportError{Msg: "deadline exceeded"}
	if sdkerrors.IsRetryable(err) {
		t.Fatal("transport error must require read-back")
	}
	if !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatal("transport error must preserve unknown mutation outcome")
	}
}

func TestMapConnectErrorSurfacesAuthInternalError(t *testing.T) {
	mapped := mapAuthDetail(t, authv1.AuthErrorCode_AUTH_INTERNAL_ERROR, "internal")
	if sdkerrors.AuthErrorCode(mapped) != sdkerrors.AuthCodeInternalError {
		t.Fatalf("AuthErrorCode=%q", sdkerrors.AuthErrorCode(mapped))
	}
}

func TestMapConnectErrorSurfacesStableMFACodes(t *testing.T) {
	cases := []struct {
		code  authv1.AuthErrorCode
		want  string
		check func(error) bool
	}{
		{authv1.AuthErrorCode_AUTH_MFA_NOT_ENROLLED, sdkerrors.AuthCodeMFANotEnrolled, sdkerrors.IsMFAEnrollmentRequired},
		{authv1.AuthErrorCode_AUTH_STEP_UP_REQUIRED, sdkerrors.AuthCodeStepUpRequired, sdkerrors.IsStepUpRequired},
		{authv1.AuthErrorCode_AUTH_MFA_ELEVATION_REQUIRED, sdkerrors.AuthCodeMFAElevationRequired, sdkerrors.IsMFAElevationRequired},
		{authv1.AuthErrorCode_AUTH_MFA_LAST_FACTOR_REQUIRED, sdkerrors.AuthCodeMFALastFactorRequired, sdkerrors.IsMFALastFactorRequired},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			mapped := mapAuthDetail(t, tc.code, "mfa control flow")
			if !tc.check(mapped) {
				t.Fatalf("predicate false for mapped error %#v", mapped)
			}
			if sdkerrors.AuthErrorCode(mapped) != tc.want {
				t.Fatalf("AuthErrorCode=%q", sdkerrors.AuthErrorCode(mapped))
			}
		})
	}
}

func TestMapConnectErrorSurfacesWithdrawAndTransferDetails(t *testing.T) {
	withdrawErr := connect.NewError(connect.CodeFailedPrecondition, errors.New("insufficient"))
	withdrawDetail, err := connect.NewErrorDetail(&withdrawv1.ErrorDetail{
		Code: withdrawv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS,
	})
	if err != nil {
		t.Fatal(err)
	}
	withdrawErr.AddDetail(withdrawDetail)
	mapped := MapConnectError(withdrawErr)
	apiErr, ok := mapped.(*sdkerrors.APIError)
	if !ok {
		t.Fatalf("withdraw mapped=%T %#v", mapped, mapped)
	}
	if apiErr.Code != "ERROR_CODE_INSUFFICIENT_FUNDS" {
		t.Fatalf("withdraw code=%q", apiErr.Code)
	}

	transferErr := connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
	transferDetail, err := connect.NewErrorDetail(&transferv1.ErrorDetail{
		Code: transferv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED,
	})
	if err != nil {
		t.Fatal(err)
	}
	transferErr.AddDetail(transferDetail)
	mapped = MapConnectError(transferErr)
	if _, ok := mapped.(*sdkerrors.AuthError); !ok {
		t.Fatalf("transfer mapped=%T %#v", mapped, mapped)
	}

	expiredErr := connect.NewError(connect.CodeFailedPrecondition, errors.New("expired"))
	expiredDetail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_CANCEL_REQUEST_EXPIRED,
	})
	if err != nil {
		t.Fatal(err)
	}
	expiredErr.AddDetail(expiredDetail)
	mapped = MapConnectError(expiredErr)
	apiErr, ok = mapped.(*sdkerrors.APIError)
	if !ok || apiErr.Code != "ERROR_CODE_CANCEL_REQUEST_EXPIRED" {
		t.Fatalf("expired mapped=%T %#v", mapped, mapped)
	}
}

func TestMapConnectErrorSurfacesAuthTermsNotAccepted(t *testing.T) {
	mapped := mapAuthDetail(t, authv1.AuthErrorCode_AUTH_TERMS_NOT_ACCEPTED, "terms")
	apiErr, ok := mapped.(*sdkerrors.APIError)
	if !ok {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if apiErr.Code != "AUTH_TERMS_NOT_ACCEPTED" {
		t.Fatalf("code=%q", apiErr.Code)
	}
	if apiErr.Msg != "terms" {
		t.Fatalf("msg=%q", apiErr.Msg)
	}
}

func mapAuthDetail(t *testing.T, code authv1.AuthErrorCode, message string) error {
	t.Helper()
	connectErr := connect.NewError(connect.CodePermissionDenied, nil)
	detail, err := connect.NewErrorDetail(&authv1.AuthErrorDetail{
		Code:    code,
		Message: message,
	})
	if err != nil {
		t.Fatalf("NewErrorDetail: %v", err)
	}
	connectErr.AddDetail(detail)
	return MapConnectError(connectErr)
}

func TestMapConnectErrorOrderOutcomeUnknownRequiresReconciliation(t *testing.T) {
	connectErr := connect.NewError(connect.CodeDeadlineExceeded, errors.New("cancel not confirmed in time"))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_ORDER_CANCEL_OUTCOME_UNKNOWN})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	if !sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatalf("mapped=%T %v must report an unknown mutation outcome", mapped, mapped)
	}
}

func TestMapConnectErrorCreateOutcomeUnknownRequiresReconciliation(t *testing.T) {
	for _, code := range []orderv1.ErrorCode{
		orderv1.ErrorCode_ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN,
		orderv1.ErrorCode_ERROR_CODE_TRIGGER_CREATE_OUTCOME_UNKNOWN,
		orderv1.ErrorCode_ERROR_CODE_TRIGGER_CONTROL_OUTCOME_UNKNOWN,
	} {
		t.Run(code.String(), func(t *testing.T) {
			connectErr := connect.NewError(connect.CodeDeadlineExceeded, errors.New("create not confirmed in time"))
			detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			connectErr.AddDetail(detail)
			mapped := MapConnectError(connectErr)
			apiErr, ok := mapped.(*sdkerrors.APIError)
			if !ok || apiErr.Code != code.String() {
				t.Fatalf("mapped=%T %#v", mapped, mapped)
			}
			if !sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("%s must report an unknown mutation outcome", code)
			}
			if sdkerrors.IsRetryable(mapped) {
				t.Fatalf("%s must not be retried before reconciliation", code)
			}
		})
	}
}

func TestMapConnectErrorAuthTemporarilyUnavailableIsRetryable(t *testing.T) {
	mapped := mapAuthDetail(t, authv1.AuthErrorCode_AUTH_TEMPORARILY_UNAVAILABLE, "try again")
	apiErr, ok := mapped.(*sdkerrors.APIError)
	if !ok || apiErr.Code != sdkerrors.AuthCodeTemporarilyUnavailable {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if !sdkerrors.IsRetryable(mapped) {
		t.Fatal("AUTH_TEMPORARILY_UNAVAILABLE changed nothing and is retryable")
	}
	if sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatal("AUTH_TEMPORARILY_UNAVAILABLE changed nothing; the outcome is known")
	}
}

// Refusals retain their code and known outcome. TWAP capacity explicitly
// permits a later retry; the other listed refusals require caller action.
func TestMapConnectErrorOrderRefusalRetryGuidance(t *testing.T) {
	for _, tc := range []struct {
		code       orderv1.ErrorCode
		connectErr connect.Code
	}{
		{orderv1.ErrorCode_ERROR_CODE_ATTACHED_LEGS_EXCEEDED, connect.CodeFailedPrecondition},
		{orderv1.ErrorCode_ERROR_CODE_FEE_ASSET_INVALID, connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_FEE_CEILING_INVALID, connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_UNSUPPORTED_FIELD_SET, connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_TRIGGER_FIRE_LIMIT_REACHED, connect.CodeFailedPrecondition},
		{orderv1.ErrorCode_ERROR_CODE_CONDITION_FLAGS_INVALID, connect.CodeInvalidArgument},
		{orderv1.ErrorCode_ERROR_CODE_CONDITION_PARAMETER_SET, connect.CodeInvalidArgument},
		// TWAP capacity can recover and carries retry-later guidance.
		{orderv1.ErrorCode_ERROR_CODE_TWAP_CAPACITY_REACHED, connect.CodeResourceExhausted},
		{orderv1.ErrorCode_ERROR_CODE_CHILD_ORDER_INCONSISTENT, connect.CodeFailedPrecondition},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			connectErr := connect.NewError(tc.connectErr, errors.New("refused"))
			detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: tc.code})
			if err != nil {
				t.Fatal(err)
			}
			connectErr.AddDetail(detail)
			mapped := MapConnectError(connectErr)
			if tc.code == orderv1.ErrorCode_ERROR_CODE_TWAP_CAPACITY_REACHED {
				var rate *sdkerrors.RateLimitError
				if !errors.As(mapped, &rate) || rate.Code != tc.code.String() || !sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
					t.Fatalf("%T %+v", mapped, mapped)
				}
				return
			}
			apiErr, ok := mapped.(*sdkerrors.APIError)
			if !ok || apiErr.Code != tc.code.String() {
				t.Fatalf("mapped=%T %#v", mapped, mapped)
			}
			if sdkerrors.IsRetryable(mapped) {
				t.Fatalf("%s must not be retryable", tc.code)
			}
			if sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("%s is a contract-defined refusal; the outcome is known", tc.code)
			}
		})
	}
}

// Member calls that target the sub-account's owner answer failed_precondition
// with AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED. The typed error keeps the auth code
// readable through *APIError, and it is final.
func TestMapConnectErrorTypesSubaccountOwnerRoleLocked(t *testing.T) {
	connectErr := connect.NewError(connect.CodeFailedPrecondition, errors.New("owner role is locked"))
	detail, err := connect.NewErrorDetail(&authv1.AuthErrorDetail{
		Code:    authv1.AuthErrorCode_AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED,
		Message: "The owner's role can't be changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)

	var locked *sdkerrors.SubaccountOwnerRoleLockedError
	if !errors.As(mapped, &locked) || !sdkerrors.IsSubaccountOwnerRoleLocked(mapped) {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	if locked.Msg != "The owner's role can't be changed" {
		t.Fatalf("msg=%q", locked.Msg)
	}
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.Code != sdkerrors.AuthCodeSubaccountOwnerRoleLocked {
		t.Fatalf("APIError view=%#v", apiErr)
	}
	if sdkerrors.AuthErrorCode(mapped) != "AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED" {
		t.Fatalf("auth code=%q", sdkerrors.AuthErrorCode(mapped))
	}
	if !errors.Is(mapped, sdkerrors.ErrPolyester) || sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatal("owner role lock is a final polyester error")
	}
	if sdkerrors.IsSubaccountOwnerRoleLocked(mapAuthDetail(t, authv1.AuthErrorCode_AUTH_REVISION_CONFLICT, "x")) {
		t.Fatal("other auth codes are not the owner role lock")
	}
}
