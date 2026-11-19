package transport

import (
	"errors"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	polyesterapiv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/api/v1"
)

// A ConnectRPC request over the size limit carries the typed
// RequestErrorDetail: it is the non-retryable REQUEST_TOO_LARGE refusal, with
// the limit, never a rate refusal. Without the detail, or with an unknown
// code, a resource_exhausted stays a rate refusal.
func TestRequestTooLargeDetailIsTheTypedSizeRefusal(t *testing.T) {
	err := callMeAgainst(t, http.StatusTooManyRequests, "application/json", connectDetailBody(t, "resource_exhausted", "request too large",
		"polyester.api.v1.RequestErrorDetail", &polyesterapiv1.RequestErrorDetail{Code: polyesterapiv1.RequestErrorCode_REQUEST_TOO_LARGE, LimitBytes: 65536}))
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != sdkerrors.CodeRequestTooLarge || api.LimitBytes != 65536 || !sdkerrors.IsRequestTooLarge(err) {
		t.Fatalf("err = %T %+v, want an APIError REQUEST_TOO_LARGE with its limit", err, err)
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v, want both false", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
	for name, body := range map[string]string{
		"no detail": `{"code":"resource_exhausted","message":"request too large"}`,
		// The message text is never the signal, even when it is the code.
		"code as the message": `{"code":"resource_exhausted","message":"REQUEST_TOO_LARGE"}`,
		"code in the message": `{"code":"resource_exhausted","message":"REQUEST_TOO_LARGE: limit 65536 bytes"}`,
		"unknown code":        connectDetailBody(t, "resource_exhausted", "request too large", "polyester.api.v1.RequestErrorDetail", &polyesterapiv1.RequestErrorDetail{Code: 9}),
	} {
		err := callMeAgainst(t, http.StatusTooManyRequests, "application/json", body)
		var rate *sdkerrors.RateLimitError
		if !errors.As(err, &rate) || sdkerrors.IsRequestTooLarge(err) {
			t.Fatalf("%s: err = %T %+v, want the rate refusal", name, err, err)
		}
	}
}

// A typed REQUEST_TOO_LARGE refusal without its limit breaks the contract,
// which sets limit_bytes on every such refusal: it is no size refusal to act
// on, and it keeps the answer's status and Retry-After.
func TestRequestTooLargeDetailWithoutItsLimitFailsClosed(t *testing.T) {
	err := callMeAgainst(t, http.StatusTooManyRequests, "application/json", connectDetailBody(t, "resource_exhausted", "request too large",
		"polyester.api.v1.RequestErrorDetail", &polyesterapiv1.RequestErrorDetail{Code: polyesterapiv1.RequestErrorCode_REQUEST_TOO_LARGE}),
		map[string]string{"Retry-After": "7"})
	var contract *sdkerrors.ResponseContractError
	if !errors.As(err, &contract) || sdkerrors.IsRequestTooLarge(err) {
		t.Fatalf("err = %T %+v, want a ResponseContractError", err, err)
	}
	if sdkerrors.IsRetryable(err) {
		t.Fatal("a malformed refusal must not be retried")
	}
	if status := sdkerrors.AnswerStatus(err); status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", status)
	}
	if delay := sdkerrors.RetryAfter(err); delay == nil || *delay != 7 {
		t.Fatalf("retry after = %v, want 7", delay)
	}
}
