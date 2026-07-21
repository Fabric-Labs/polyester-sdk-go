package wire

import (
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestMapConnectErrorSurfacesAuthRevisionConflict(t *testing.T) {
	connectErr := connect.NewError(connect.CodeAborted, nil)
	detail, err := connect.NewErrorDetail(&authv1.AuthErrorDetail{
		Code:    authv1.AuthErrorCode_AUTH_REVISION_CONFLICT,
		Message: "resource changed",
	})
	if err != nil {
		t.Fatalf("NewErrorDetail: %v", err)
	}
	connectErr.AddDetail(detail)

	mapped := MapConnectError(connectErr)
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
