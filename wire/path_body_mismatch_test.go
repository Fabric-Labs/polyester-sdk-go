package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

// A path and body that name different resources is its own validation code,
// with its field violation, not a generic validation failure or a code the
// SDK cannot name.
func TestOrderPathBodyIDMismatchKeepsItsCode(t *testing.T) {
	order := &orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_PATH_BODY_ID_MISMATCH,
		Violations: []*orderv1.FieldViolation{{
			FieldPath: "order_id",
			RuleId:    "path_body_id_mismatch",
			Message:   "order_id in the body differs from the path",
		}},
	}
	connectErr := connect.NewError(connect.CodeInvalidArgument, errors.New("order_id in the body differs from the path"))
	detail, err := connect.NewErrorDetail(order)
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(mapped, &validationErr) || validationErr.Code != "ERROR_CODE_PATH_BODY_ID_MISMATCH" {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	got := sdkerrors.Violations(mapped)
	if len(got) != 1 || got[0].FieldPath != "order_id" || got[0].RuleID != "path_body_id_mismatch" {
		t.Fatalf("violations=%#v", got)
	}
	if sdkerrors.IsRetryable(mapped) {
		t.Fatal("a mismatch is not retryable")
	}
	encoded, err := proto.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	if !CarriesContractCode(string(order.ProtoReflect().Descriptor().FullName()), encoded) ||
		FutureCodeLabel(string(order.ProtoReflect().Descriptor().FullName()), encoded) != "" {
		t.Fatal("ERROR_CODE_PATH_BODY_ID_MISMATCH should be a known contract code")
	}
}

func TestAuthPathBodyIDMismatchKeepsItsCode(t *testing.T) {
	connectErr := connect.NewError(connect.CodeInvalidArgument, errors.New("policy id in the body differs from the path"))
	detail, err := connect.NewErrorDetail(&authv1.AuthErrorDetail{
		Code:    authv1.AuthErrorCode_AUTH_PATH_BODY_ID_MISMATCH,
		Message: "policy id in the body differs from the path",
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	if code := sdkerrors.AuthErrorCode(mapped); code != "AUTH_PATH_BODY_ID_MISMATCH" {
		t.Fatalf("code=%q mapped=%T %#v", code, mapped, mapped)
	}
	if sdkerrors.IsRetryable(mapped) || sdkerrors.IsSessionRequired(mapped) {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
}

// A key whose policy lacks an account permission is refused with
// AUTH_API_KEY_ACCESS_DENIED, and the message names the permission to grant.
func TestMissingAccountPermissionNamesThePermission(t *testing.T) {
	message := "api key policy does not allow READ_VIP_STATUS"
	mapped := connectAuthError(t, connect.CodePermissionDenied, authv1.AuthErrorCode_AUTH_API_KEY_ACCESS_DENIED, message)
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.Msg != message || sdkerrors.AuthErrorCode(mapped) != "AUTH_API_KEY_ACCESS_DENIED" {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
	if sdkerrors.IsRetryable(mapped) || sdkerrors.IsSessionRequired(mapped) {
		t.Fatalf("mapped=%T %#v", mapped, mapped)
	}
}
