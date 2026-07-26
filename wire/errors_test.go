package wire

import (
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
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
	if !sdkerrors.IsRetryable(mapped) {
		t.Fatal("rate limit must be retryable")
	}
	if sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatal("rate-limit rejection must not be marked as ambiguous")
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
