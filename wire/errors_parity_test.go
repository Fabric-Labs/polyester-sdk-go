package wire

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

func TestMapConnectErrorTimestampSkewIsRetryable(t *testing.T) {
	mapped := MapConnectError(connect.NewError(connect.CodeUnauthenticated, errors.New("TIMESTAMP_SKEW")))
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(mapped, &skew) {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	if skew.Status != 401 || !errors.Is(mapped, sdkerrors.ErrPolyester) {
		t.Fatalf("skew=%+v", skew)
	}
	if !sdkerrors.IsRetryable(mapped) {
		t.Fatal("a skewed signature changed nothing; retrying signs again")
	}
	if sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatal("a request rejected at signature verification has a known outcome")
	}
	// Ordinary authentication failures stay non-retryable auth errors.
	var authErr *sdkerrors.AuthError
	if other := MapConnectError(connect.NewError(connect.CodeUnauthenticated, errors.New("bad signature"))); !errors.As(other, &authErr) || sdkerrors.IsRetryable(other) {
		t.Fatalf("other=%T %v", other, other)
	}
}

func TestMapConnectErrorTimestampSkewSurvivesConnectWrapping(t *testing.T) {
	// The transport reports a problem+json skew rejection as a client error,
	// which Connect wraps before the service layer maps it.
	wrapped := connect.NewError(connect.CodeUnknown, &sdkerrors.TimestampSkewError{Msg: "skew", Status: 401})
	var skew *sdkerrors.TimestampSkewError
	if mapped := MapConnectError(wrapped); !errors.As(mapped, &skew) || skew.Status != 401 {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
}

func TestMapConnectErrorUnimplementedIsNotImplemented(t *testing.T) {
	for name, err := range map[string]error{
		"connect unimplemented": connect.NewError(connect.CodeUnimplemented, errors.New("trigger ladders are not enabled")),
		"bare HTTP 501":         connect.NewError(connect.CodeUnknown, errors.New("501 Not Implemented")),
	} {
		mapped := MapConnectError(err)
		var notImplemented *sdkerrors.NotImplementedError
		if !errors.As(mapped, &notImplemented) || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s: mapped=%T %v", name, mapped, mapped)
		}
	}
	var routeErr *sdkerrors.RouteNotFoundError
	if mapped := MapConnectError(connect.NewError(connect.CodeUnimplemented, errors.New("404 Not Found"))); !errors.As(mapped, &routeErr) {
		t.Fatalf("bare 404 must stay RouteNotFoundError, got %T", mapped)
	}
}

func TestMapConnectErrorDuplicateClientOrderIDKeepsExistingOrderID(t *testing.T) {
	connectErr := connect.NewError(connect.CodeAlreadyExists, errors.New("duplicate client order id"))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code:            orderv1.ErrorCode_ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID,
		ExistingOrderId: proto.Uint64(77),
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	var apiErr *sdkerrors.APIError
	mapped := MapConnectError(connectErr)
	if !errors.As(mapped, &apiErr) || apiErr.Code != "ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID" {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	if apiErr.ExistingOrderID != codecs.FormatUint64ID(77) {
		t.Fatalf("existing order id=%q", apiErr.ExistingOrderID)
	}
}
