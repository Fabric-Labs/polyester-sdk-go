package transport

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// A timestamp-skew refusal carries the window the server checked the
// timestamp against, from the REST problem's timestampWindow member or the
// ConnectRPC AuthErrorDetail, and stays retryable.
func TestTimestampSkewRefusalCarriesItsWindow(t *testing.T) {
	want := sdkerrors.TimestampWindow{ServerTimeMs: 1763337600000, RecvWindowMs: 5000, MaxFutureMs: 1000}
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"rest", "application/problem+json", `{"type":"https://polyester.dev/problems/timestamp-skew","title":"API Key Timestamp Skew","status":401,` +
			`"detail":"API key timestamp is outside the request's receive window","code":"TIMESTAMP_SKEW",` +
			`"timestampWindow":{"serverTimeMs":"1763337600000","recvWindowMs":5000,"maxFutureMs":1000}}`},
		{"connect", "application/json", connectDetailBody(t, "unauthenticated", "API key timestamp is outside the request's receive window", "auth.v1.AuthErrorDetail",
			&authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_TIMESTAMP_SKEW, TimestampWindow: &authv1.TimestampWindow{ServerTimeMs: 1763337600000, RecvWindowMs: 5000, MaxFutureMs: 1000}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := callMeAgainst(t, http.StatusUnauthorized, tc.contentType, tc.body)
			var skew *sdkerrors.TimestampSkewError
			if !errors.As(err, &skew) || skew.Window == nil || *skew.Window != want {
				t.Fatalf("err = %T %+v, want a TimestampSkewError with window %+v", err, err, want)
			}
			if !sdkerrors.IsRetryable(err) {
				t.Fatal("TIMESTAMP_SKEW is retryable")
			}
		})
	}
	// A refusal without a window, or with one that does not parse exactly,
	// carries none.
	for _, body := range []string{
		`{"status":401,"code":"TIMESTAMP_SKEW"}`,
		`{"status":401,"code":"TIMESTAMP_SKEW","timestampWindow":{"serverTimeMs":"1.5","recvWindowMs":5000,"maxFutureMs":1000}}`,
		`{"status":401,"code":"TIMESTAMP_SKEW","timestampWindow":{"serverTimeMs":"1","recvWindowMs":-1,"maxFutureMs":1000}}`,
	} {
		err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json", body)
		var skew *sdkerrors.TimestampSkewError
		if !errors.As(err, &skew) || skew.Window != nil {
			t.Fatalf("%s: err = %T %+v, want a TimestampSkewError without a window", body, err, err)
		}
	}
}

// The REST refusals the APIs now answer in uppercase, on a ConnectRPC call
// refused before its method runs, keep their meaning: a request refused for
// its body is definite and not retryable, and a step-up or MFA refusal reads
// as such.
func TestUppercaseRESTRefusalsKeepTheirMeaning(t *testing.T) {
	problem := func(status int, code string) string {
		return fmt.Sprintf(`{"type":"https://polyester.dev/problems/x","title":"t","status":%d,"detail":"d","code":%q}`, status, code)
	}
	for _, tc := range []struct {
		status int
		code   string
		check  func(error) bool
	}{
		{http.StatusBadRequest, "BAD_REQUEST", func(err error) bool { return !sdkerrors.IsRetryable(err) && !sdkerrors.MutationOutcomeUnknown(err) }},
		{http.StatusBadRequest, "INVALID_REQUEST", func(err error) bool { return !sdkerrors.IsRetryable(err) && !sdkerrors.MutationOutcomeUnknown(err) }},
		{http.StatusNotFound, "NOT_FOUND", func(err error) bool { return !sdkerrors.IsRetryable(err) && !sdkerrors.MutationOutcomeUnknown(err) }},
		{http.StatusForbidden, "PERMISSION_DENIED", func(err error) bool {
			var auth *sdkerrors.AuthError
			return errors.As(err, &auth) && auth.Code == "PERMISSION_DENIED"
		}},
		{http.StatusForbidden, "AUTH_STEP_UP_REQUIRED", sdkerrors.IsStepUpRequired},
		{http.StatusForbidden, "AUTH_MFA_ELEVATION_REQUIRED", sdkerrors.IsMFAElevationRequired},
		{http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", func(err error) bool {
			var server *sdkerrors.ServerError
			return errors.As(err, &server) && server.Code == "SERVICE_UNAVAILABLE" && server.Status == http.StatusServiceUnavailable
		}},
		{http.StatusServiceUnavailable, "UPSTREAM_ERROR", func(err error) bool {
			var server *sdkerrors.ServerError
			return errors.As(err, &server) && server.Code == "UPSTREAM_ERROR"
		}},
	} {
		err := callMeAgainst(t, tc.status, "application/problem+json", problem(tc.status, tc.code))
		if !tc.check(err) {
			t.Fatalf("%d %s: err = %T %+v", tc.status, tc.code, err, err)
		}
	}
}
