package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
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
	if !sdkerrors.IsRetryable(err) {
		t.Fatal("transport error must be retryable")
	}
	if !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatal("transport error must preserve unknown mutation outcome")
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
