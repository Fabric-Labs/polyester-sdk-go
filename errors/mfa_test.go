package errors_test

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestAuthErrorCodePredicates(t *testing.T) {
	cases := []struct {
		name  string
		code  string
		check func(error) bool
	}{
		{"enrollment", sdkerrors.AuthCodeMFANotEnrolled, sdkerrors.IsMFAEnrollmentRequired},
		{"step-up", sdkerrors.AuthCodeStepUpRequired, sdkerrors.IsStepUpRequired},
		{"elevation", sdkerrors.AuthCodeMFAElevationRequired, sdkerrors.IsMFAElevationRequired},
		{"last-factor", sdkerrors.AuthCodeMFALastFactorRequired, sdkerrors.IsMFALastFactorRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &sdkerrors.APIError{Msg: "mfa", Code: tc.code}
			if !tc.check(err) {
				t.Fatalf("expected predicate true for code %q", tc.code)
			}
			if sdkerrors.AuthErrorCode(err) != tc.code {
				t.Fatalf("AuthErrorCode=%q", sdkerrors.AuthErrorCode(err))
			}
			for _, other := range cases {
				if other.code == tc.code {
					continue
				}
				if other.check(err) {
					t.Fatalf("predicate for %q matched code %q", other.name, tc.code)
				}
			}
		})
	}
}

func TestAuthErrorCodeIgnoresNonAPIErrorsAndMessageText(t *testing.T) {
	if sdkerrors.AuthErrorCode(errors.New("must enroll mfa")) != "" {
		t.Fatal("plain errors must not expose auth codes")
	}
	if sdkerrors.IsMFAEnrollmentRequired(&sdkerrors.AuthError{Msg: "must enroll mfa"}) {
		t.Fatal("message heuristics must not classify MFA enrollment")
	}
	if sdkerrors.IsStepUpRequired(&sdkerrors.APIError{Msg: "step-up required", Code: "permission_denied"}) {
		t.Fatal("non-auth codes must not classify step-up")
	}
	// Removed pre-release code must not be treated as enrollment.
	if sdkerrors.IsMFAEnrollmentRequired(&sdkerrors.APIError{
		Msg:  "api key mfa",
		Code: "AUTH_API_KEY_MFA_REQUIRED",
	}) {
		t.Fatal("AUTH_API_KEY_MFA_REQUIRED must not map to enrollment")
	}
}
