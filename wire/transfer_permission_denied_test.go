package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
)

// GetInternalTransferBatch refuses an API key whose policy does not grant
// READ_INTERNAL_TRANSFERS with permission_denied, naming the action to grant,
// rather than not_found: it maps to an *errors.AuthError carrying the
// PERMISSION_DENIED code and that message, final rather than retryable.
func TestInternalTransferPermissionDeniedNamesTheMissingGrant(t *testing.T) {
	const msg = "API key policy does not grant READ_INTERNAL_TRANSFERS"
	refusal := connect.NewError(connect.CodePermissionDenied, errors.New(msg))
	d, err := connect.NewErrorDetail(&transferv1.ErrorDetail{Code: transferv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED})
	if err != nil {
		t.Fatal(err)
	}
	refusal.AddDetail(d)
	mapped := MapConnectError(refusal)
	var authErr *sdkerrors.AuthError
	if !errors.As(mapped, &authErr) {
		t.Fatalf("mapped = %T %#v, want an *errors.AuthError", mapped, mapped)
	}
	if authErr.Code != transferv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED.String() || authErr.Msg != msg ||
		sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatalf("mapped = %#v retryable=%v unknown=%v", authErr, sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
	}
}
