package transport

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

func connectDetailBody(t *testing.T, code, message, typeName string, detail proto.Message) string {
	t.Helper()
	data, err := proto.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	return `{"code":"` + code + `","message":"` + message + `","details":[{"type":"` + typeName + `","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}]}`
}

// AUTH_REPLAY_CAPACITY_EXHAUSTED refuses a signed request before it is
// processed, because the key's (or account's) share of the replay record is
// full until earlier requests' receive windows end. The credentials are
// valid and nothing changed, so it is a retryable rate refusal on both
// protocols, never an authentication failure or an unknown outcome.
func TestReplayCapacityExhaustedIsARetryableRateRefusal(t *testing.T) {
	const message = "replay capacity exhausted for this API key"
	for _, tc := range []struct {
		name, contentType, body string
		headers                 map[string]string
		retryAfter              *float64
	}{
		{name: "connect", contentType: "application/json", body: connectDetailBody(t, "resource_exhausted", message, "auth.v1.AuthErrorDetail",
			&authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_REPLAY_CAPACITY_EXHAUSTED, Message: message})},
		{name: "rest", contentType: "application/problem+json", body: `{"status":429,"code":"REPLAY_CAPACITY_EXHAUSTED","title":"Too Many Requests","detail":"` + message + `"}`},
		{name: "rest with Retry-After", contentType: "application/problem+json", body: `{"status":429,"code":"REPLAY_CAPACITY_EXHAUSTED"}`,
			headers: map[string]string{"Retry-After": "1"}, retryAfter: new(1.0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := callMeAgainst(t, http.StatusTooManyRequests, tc.contentType, tc.body, tc.headers)
			var rate *sdkerrors.RateLimitError
			if !errors.As(mapped, &rate) {
				t.Fatalf("error = %T %v, want *errors.RateLimitError", mapped, mapped)
			}
			if rate.Code != "AUTH_REPLAY_CAPACITY_EXHAUSTED" || sdkerrors.AuthErrorCode(mapped) != "AUTH_REPLAY_CAPACITY_EXHAUSTED" {
				t.Fatalf("code = %q, AuthErrorCode = %q", rate.Code, sdkerrors.AuthErrorCode(mapped))
			}
			if !sdkerrors.IsReplayCapacityExhausted(mapped) || rate.Code != sdkerrors.AuthCodeReplayCapacityExhausted {
				t.Fatal("IsReplayCapacityExhausted = false")
			}
			var authErr *sdkerrors.AuthError
			if errors.As(mapped, &authErr) {
				t.Fatal("replay capacity is not a credentials problem")
			}
			if !sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("retryable=%v outcome unknown=%v, want a retryable known refusal", sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
			}
			switch {
			case tc.retryAfter == nil && rate.RetryAfter != nil:
				t.Fatalf("retry after = %v, want none: the refusal carries no hint", *rate.RetryAfter)
			case tc.retryAfter != nil && (rate.RetryAfter == nil || *rate.RetryAfter != *tc.retryAfter):
				t.Fatalf("retry after = %v, want %v", rate.RetryAfter, *tc.retryAfter)
			}
		})
	}
}

// AUTH_RECV_WINDOW_INVALID refuses an X-API-RECV-WINDOW header that is not
// one whole number from 1 to 60000: an API-key refusal the caller must fix.
func TestRecvWindowInvalidIsATerminalAuthRefusal(t *testing.T) {
	const message = "X-API-RECV-WINDOW must be an integer from 1 to 60000"
	for _, tc := range []struct{ name, contentType, body string }{
		{"connect", "application/json", connectDetailBody(t, "unauthenticated", message, "auth.v1.AuthErrorDetail",
			&authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_RECV_WINDOW_INVALID, Message: message})},
		{"rest", "application/problem+json", `{"status":401,"code":"RECV_WINDOW_INVALID","title":"Unauthorized","detail":"` + message + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := callMeAgainst(t, http.StatusUnauthorized, tc.contentType, tc.body)
			var authErr *sdkerrors.AuthError
			if !errors.As(mapped, &authErr) || authErr.Status != http.StatusUnauthorized {
				t.Fatalf("error = %T %+v, want *errors.AuthError with status 401", mapped, mapped)
			}
			if got := sdkerrors.AuthErrorCode(mapped); got != sdkerrors.AuthCodeRecvWindowInvalid {
				t.Fatalf("AuthErrorCode = %q", got)
			}
			if !sdkerrors.IsRecvWindowInvalid(mapped) || sdkerrors.IsReplayCapacityExhausted(mapped) {
				t.Fatal("IsRecvWindowInvalid = false")
			}
			if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("retryable=%v outcome unknown=%v, want a terminal refusal", sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
			}
		})
	}
}

// ERROR_CODE_REQUEST_DEADLINE_EXCEEDED (308) answers an order or trigger
// write whose receive window ended before it was sent on for admission.
// Nothing changed, so it is retryable after signing again, and never an
// unknown outcome.
func TestRequestDeadlineExceededIsRetryableAndKnown(t *testing.T) {
	const message = "request deadline exceeded before admission"
	for _, tc := range []struct{ name, contentType, body string }{
		{"connect", "application/json", connectDetailBody(t, "failed_precondition", message, "orders.v1.ErrorDetail",
			&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_REQUEST_DEADLINE_EXCEEDED})},
		{"rest", "application/problem+json", `{"status":400,"code":"REQUEST_DEADLINE_EXCEEDED","title":"Bad Request","detail":"` + message + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := callMeAgainst(t, http.StatusBadRequest, tc.contentType, tc.body)
			var api *sdkerrors.APIError
			if !errors.As(mapped, &api) || api.Code != sdkerrors.CodeRequestDeadlineExceeded || !sdkerrors.IsRequestDeadlineExceeded(mapped) {
				t.Fatalf("error = %T %+v, want *errors.APIError ERROR_CODE_REQUEST_DEADLINE_EXCEEDED", mapped, mapped)
			}
			if !sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("retryable=%v outcome unknown=%v, want retryable and known", sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
			}
		})
	}
}
