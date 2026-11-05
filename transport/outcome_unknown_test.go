package transport

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

func TestRESTOutcomeUnknownMatchesConnect(t *testing.T) {
	for _, name := range orderv1.ErrorCode_name {
		if !strings.HasSuffix(name, "_OUTCOME_UNKNOWN") {
			continue
		}
		for _, status := range []int{504, 503} {
			t.Run(fmt.Sprintf("%s/%d", name, status), func(t *testing.T) {
				err := callMeAgainst(t, status, "application/problem+json",
					fmt.Sprintf(`{"code":%q,"detail":"read back first"}`, strings.TrimPrefix(name, "ERROR_CODE_")))
				api, ok := err.(*sdkerrors.APIError)
				if !ok || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
					t.Fatalf("class=%T retryable=%v outcomeUnknown=%v: %v", err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err), err)
				}
				if api.Code != name || api.Msg != "read back first" {
					t.Fatalf("API error: %+v", api)
				}
			})
		}
	}
}

func TestUnrecognisedREST504RemainsAmbiguousTransportFailure(t *testing.T) {
	for _, tc := range []struct{ contentType, body string }{
		{"text/html", "<html>Gateway Timeout</html>"},
		{"application/problem+json", `{"code":"UNKNOWN_GATEWAY_FAILURE"}`},
	} {
		err := callMeAgainst(t, 504, tc.contentType, tc.body)
		var server *sdkerrors.ServerError
		if !errors.As(err, &server) || !sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("class=%T retryable=%v outcomeUnknown=%v", err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
	}
}
