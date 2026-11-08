package transport

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/proto"
)

func errorCode(err error) string {
	var api *sdkerrors.APIError
	var validation *sdkerrors.ValidationError
	var server *sdkerrors.ServerError
	var rate *sdkerrors.RateLimitError
	switch {
	case errors.As(err, &validation):
		return validation.Code
	case errors.As(err, &rate):
		return rate.Code
	case errors.As(err, &server):
		return server.Code
	case errors.As(err, &api):
		return api.Code
	}
	return ""
}

// A refusal reads the same ERROR_CODE_ code over REST and ConnectRPC: the REST
// problem names it without the prefix, the typed detail by its enum value.
func TestRefusalCodeIsTheSameOverRESTAndConnect(t *testing.T) {
	for _, tc := range []struct {
		name       string
		detailType string
		detail     proto.Message
		status     int
		connect    string
	}{
		{"SAME_SOURCE_DESTINATION", "transfer.v1.ErrorDetail", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_SAME_SOURCE_DESTINATION}, 400, "invalid_argument"},
		{"INSUFFICIENT_FUNDS", "transfer.v1.ErrorDetail", &transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS}, 400, "invalid_argument"},
		{"AMOUNT_BELOW_MINIMUM", "chain.withdraw.v1.ErrorDetail", &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_AMOUNT_BELOW_MINIMUM}, 400, "invalid_argument"},
		{"INVALID_SUBACCOUNT_ID", "ledger.read.v1.ErrorDetail", &ledgerrdv1.ErrorDetail{Code: ledgerrdv1.ErrorCode_ERROR_CODE_INVALID_SUBACCOUNT_ID}, 400, "invalid_argument"},
		{"QTY_STEP_SIZE", "orders.v1.ErrorDetail", &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_QTY_STEP_SIZE}, 400, "invalid_argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := proto.Marshal(tc.detail)
			if err != nil {
				t.Fatal(err)
			}
			typedBody, _ := json.Marshal(map[string]any{
				"code": tc.connect, "message": "refused",
				"details": []map[string]string{{"type": tc.detailType, "value": base64.StdEncoding.EncodeToString(data)}},
			})
			restBody, _ := json.Marshal(map[string]any{"code": tc.name, "status": tc.status, "detail": "refused"})
			typed := callMeAgainst(t, tc.status, "application/json", string(typedBody))
			rest := callMeAgainst(t, tc.status, "application/problem+json", string(restBody))
			want := "ERROR_CODE_" + tc.name
			if got := errorCode(typed); got != want {
				t.Fatalf("typed ConnectRPC code = %q (%T), want %q", got, typed, want)
			}
			if got := errorCode(rest); got != want {
				t.Fatalf("REST code = %q (%T), want %q", got, rest, want)
			}
		})
	}
}

// INVALID_REQUEST is an auth, transfer and withdraw code; REST can't say which
// service refused, so it stays as sent and still reads as an auth code.
func TestRESTCodeTheAuthServiceAlsoNamesStaysAsSent(t *testing.T) {
	err := callMeAgainst(t, 400, "application/problem+json", `{"code":"INVALID_REQUEST","status":400,"detail":"refused"}`)
	if got := errorCode(err); got != "INVALID_REQUEST" {
		t.Fatalf("REST code = %q (%T), want INVALID_REQUEST", got, err)
	}
	if got := sdkerrors.AuthErrorCode(err); got != "AUTH_INVALID_REQUEST" {
		t.Fatalf("AuthErrorCode = %q, want AUTH_INVALID_REQUEST", got)
	}
}
