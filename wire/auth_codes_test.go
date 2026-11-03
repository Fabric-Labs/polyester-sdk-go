package wire

import (
	"errors"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

func connectAuthError(t *testing.T, code connect.Code, authCode authv1.AuthErrorCode, message string) error {
	t.Helper()
	connectErr := connect.NewError(code, errors.New(message))
	detail, err := connect.NewErrorDetail(&authv1.AuthErrorDetail{Code: authCode, Message: message})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	return MapConnectError(connectErr)
}

// A Connect API-key refusal carries AuthErrorDetail AUTH_TIMESTAMP_SKEW: the
// same retryable skew error as the gateway's TIMESTAMP_SKEW answer.
func TestConnectAuthTimestampSkewDetailIsRetryableSkewError(t *testing.T) {
	mapped := connectAuthError(t, connect.CodeUnauthenticated, authv1.AuthErrorCode_AUTH_TIMESTAMP_SKEW, "request timestamp is outside the allowed window")
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(mapped, &skew) || skew.Status != 401 {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	if !sdkerrors.IsRetryable(mapped) || sdkerrors.AuthErrorCode(mapped) != sdkerrors.AuthCodeTimestampSkew {
		t.Fatalf("retryable=%v code=%q", sdkerrors.IsRetryable(mapped), sdkerrors.AuthErrorCode(mapped))
	}
}

// The other API-key refusals are final auth errors that keep their reason.
func TestConnectAPIKeyRefusalDetailsAreFinalAuthErrors(t *testing.T) {
	for code, want := range map[authv1.AuthErrorCode]string{
		authv1.AuthErrorCode_AUTH_SIGNATURE_INVALID:    sdkerrors.AuthCodeSignatureInvalid,
		authv1.AuthErrorCode_AUTH_MISSING_HEADERS:      sdkerrors.AuthCodeMissingHeaders,
		authv1.AuthErrorCode_AUTH_KEY_EXPIRED:          sdkerrors.AuthCodeKeyExpired,
		authv1.AuthErrorCode_AUTH_KEY_DISABLED:         sdkerrors.AuthCodeKeyDisabled,
		authv1.AuthErrorCode_AUTH_IP_NOT_ALLOWED:       sdkerrors.AuthCodeIPNotAllowed,
		authv1.AuthErrorCode_AUTH_API_KEY_NOT_ACCEPTED: sdkerrors.AuthCodeAPIKeyNotAccepted,
	} {
		mapped := connectAuthError(t, connect.CodeUnauthenticated, code, "API key signature is invalid")
		var authErr *sdkerrors.AuthError
		if !errors.As(mapped, &authErr) || authErr.Code != want || authErr.Status != 401 || authErr.Msg != "API key signature is invalid" {
			t.Fatalf("%s: mapped=%T %#v", want, mapped, mapped)
		}
		if sdkerrors.AuthErrorCode(mapped) != want || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s: code=%q retryable=%v", want, sdkerrors.AuthErrorCode(mapped), sdkerrors.IsRetryable(mapped))
		}
	}
}

// Session-only methods refuse an account API key with permission_denied and
// AUTH_SESSION_KIND_NOT_ALLOWED, whatever the service.
func TestConnectSessionKindNotAllowedIsSessionRequired(t *testing.T) {
	mapped := connectAuthError(t, connect.CodePermissionDenied, authv1.AuthErrorCode_AUTH_SESSION_KIND_NOT_ALLOWED, "interactive session required")
	var required *sdkerrors.SessionRequiredError
	if !errors.As(mapped, &required) || !sdkerrors.IsSessionRequired(mapped) || required.Msg != "interactive session required" {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.Code != sdkerrors.AuthCodeSessionKindNotAllowed {
		t.Fatalf("APIError view=%#v", apiErr)
	}
	if sdkerrors.AuthErrorCode(mapped) != sdkerrors.AuthCodeSessionKindNotAllowed || sdkerrors.IsRetryable(mapped) ||
		!errors.Is(mapped, sdkerrors.ErrPolyester) {
		t.Fatalf("code=%q", sdkerrors.AuthErrorCode(mapped))
	}
	if sdkerrors.IsSessionRequired(mapAuthDetail(t, authv1.AuthErrorCode_AUTH_REVISION_CONFLICT, "x")) {
		t.Fatal("other auth codes do not require a session")
	}
}

// An auth code newer than the SDK decodes without failing.
func TestConnectUnknownAuthCodeStaysAPIError(t *testing.T) {
	mapped := connectAuthError(t, connect.CodeUnauthenticated, authv1.AuthErrorCode(999), "refused")
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.Msg != "refused" {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
}

// A withdraw-service refusal names the missing API-key policy action; the
// caller reads that message as the server wrote it.
func TestWithdrawPolicyRefusalMessagePassesThrough(t *testing.T) {
	const message = "api key policy does not allow READ_AUTO_WITHDRAW"
	connectErr := connect.NewError(connect.CodePermissionDenied, errors.New(message))
	detail, err := connect.NewErrorDetail(&withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	var authErr *sdkerrors.AuthError
	if !errors.As(mapped, &authErr) || authErr.Error() != message || authErr.Code != "ERROR_CODE_PERMISSION_DENIED" {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
}

func TestOrderViolationsReadThroughOneAccessor(t *testing.T) {
	connectErr := connect.NewError(connect.CodeInvalidArgument, errors.New("invalid"))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_TRIGGER_BUY_REQUIRES_LIMIT,
		Violations: []*orderv1.FieldViolation{{
			FieldPath: "order.attached_risk.trailing_stop.limit_price_ticks",
			RuleId:    "buy_required",
			Message:   "limit_price_ticks is required when the parent order sells",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	got := sdkerrors.Violations(mapped)
	if len(got) != 1 || got[0].FieldPath != "order.attached_risk.trailing_stop.limit_price_ticks" || got[0].RuleID != "buy_required" ||
		got[0].Message != "limit_price_ticks is required when the parent order sells" {
		t.Fatalf("violations=%#v", got)
	}
}

// Request validation failures carry buf.validate.Violations; their field
// paths read like ErrorDetail's.
func TestProtovalidateViolationsSurfaceAsFieldViolations(t *testing.T) {
	connectErr := connect.NewError(connect.CodeInvalidArgument, errors.New("validation error"))
	detail, err := connect.NewErrorDetail(&validate.Violations{Violations: []*validate.Violation{{
		Field: &validate.FieldPath{Elements: []*validate.FieldPathElement{
			{FieldName: proto.String("items"), Subscript: &validate.FieldPathElement_Index{Index: 2}},
			{FieldName: proto.String("limit_gtc")},
			{FieldName: proto.String("price_ticks")},
		}},
		RuleId:  proto.String("int64.gt"),
		Message: proto.String("value must be greater than 0"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	connectErr.AddDetail(detail)
	mapped := MapConnectError(connectErr)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(mapped, &validationErr) || validationErr.Code != "invalid_argument" {
		t.Fatalf("mapped=%T %v", mapped, mapped)
	}
	got := sdkerrors.Violations(mapped)
	if len(got) != 1 || got[0].FieldPath != "items[2].limit_gtc.price_ticks" || got[0].RuleID != "int64.gt" {
		t.Fatalf("violations=%#v", got)
	}
}
