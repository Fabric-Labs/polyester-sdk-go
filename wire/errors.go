package wire

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
)

var routeNotFoundMessages = map[string]struct{}{
	"not found":          {},
	"404 page not found": {},
	"404 not found":      {},
}

const emptyErrorMessage = "request failed without server error details"

func retryAfterSeconds(connectErr *connect.Error) *float64 {
	parse := func(name string, divisor float64) *float64 {
		raw := strings.TrimSpace(connectErr.Meta().Get(name))
		if raw == "" {
			return nil
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil
		}
		value /= divisor
		return &value
	}
	if seconds := parse("Retry-After", 1); seconds != nil {
		return seconds
	}
	if seconds := parse("Retry-After-Ms", 1_000); seconds != nil {
		return seconds
	}
	return parse("Grpc-Retry-Pushback-Ms", 1_000)
}

func rateLimitDetailFromProto(msg *ratelimitv1.RateLimitDetail) *sdkerrors.RateLimitDetail {
	if msg == nil {
		return nil
	}
	enumName := func(names map[int32]string, raw int32, unknownPrefix string) string {
		if name, ok := names[raw]; ok {
			return name
		}
		return fmt.Sprintf("%s(%d)", unknownPrefix, raw)
	}
	out := &sdkerrors.RateLimitDetail{
		Reason:      enumName(ratelimitv1.FailureReason_name, int32(msg.GetReason()), "UNKNOWN_FAILURE_REASON"),
		OperationID: msg.GetOperationId(),
		PolicyClass: enumName(ratelimitv1.PolicyClass_name, int32(msg.GetPolicyClass()), "UNKNOWN_POLICY_CLASS"),
		Scope:       enumName(ratelimitv1.LimiterScope_name, int32(msg.GetScope()), "UNKNOWN_LIMITER_SCOPE"),
		RefillModel: enumName(ratelimitv1.RefillModel_name, int32(msg.GetRefillModel()), "UNKNOWN_REFILL_MODEL"),
	}
	if msg.Limit != nil {
		v := msg.GetLimit()
		out.Limit = &v
	}
	if msg.Remaining != nil {
		v := msg.GetRemaining()
		out.Remaining = &v
	}
	if msg.RetryAfterMs != nil {
		v := msg.GetRetryAfterMs()
		out.RetryAfterMs = &v
	}
	if msg.PolicyVersion != nil {
		v := msg.GetPolicyVersion()
		out.PolicyVersion = &v
	}
	return out
}

func rateLimitError(msg string, detail *sdkerrors.RateLimitDetail, headerRetry *float64) *sdkerrors.RateLimitError {
	retryAfter := headerRetry
	if seconds := detail.RetryAfterSeconds(); seconds != nil {
		retryAfter = seconds
	}
	return &sdkerrors.RateLimitError{
		Msg:        msg,
		RetryAfter: retryAfter,
		Detail:     detail,
	}
}

// MapConnectError converts Connect errors into SDK error types.
func MapConnectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &sdkerrors.TransportError{Msg: err.Error()}
	}
	// Connect may wrap an interceptor-returned SDK error before it reaches the
	// service layer. Preserve client-side signing/authentication failures
	// instead of reclassifying them as transport or unknown API failures.
	var authErr *sdkerrors.AuthError
	if errors.As(err, &authErr) {
		return authErr
	}
	var skewErr *sdkerrors.TimestampSkewError
	if errors.As(err, &skewErr) {
		return skewErr
	}
	var ownerLockedErr *sdkerrors.SubaccountOwnerRoleLockedError
	if errors.As(err, &ownerLockedErr) {
		return ownerLockedErr
	}
	var sessionErr *sdkerrors.SessionRequiredError
	if errors.As(err, &sessionErr) {
		return sessionErr
	}
	var problemErr *sdkerrors.ValidationError
	if errors.As(err, &problemErr) {
		return problemErr
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return &sdkerrors.TransportError{Msg: err.Error()}
	}
	headerRetry := retryAfterSeconds(connectErr)
	if connectErr.Message() == TimestampSkewCode {
		return &sdkerrors.TimestampSkewError{
			Msg:    "API key timestamp is outside the allowed skew window",
			Status: connectHTTPStatus(connectErr.Code()),
		}
	}
	msg := connectErr.Message()
	if strings.TrimSpace(msg) == "" {
		msg = emptyErrorMessage
	}
	for _, detail := range connectErr.Details() {
		value, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if authDetail, ok := value.(*authv1.AuthErrorDetail); ok {
			codeName := authDetail.GetCode().String()
			detailMsg := authDetail.GetMessage()
			if detailMsg == "" {
				detailMsg = msg
			}
			switch authDetail.GetCode() {
			case authv1.AuthErrorCode_AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED:
				return &sdkerrors.SubaccountOwnerRoleLockedError{Msg: detailMsg}
			case authv1.AuthErrorCode_AUTH_SESSION_KIND_NOT_ALLOWED:
				return &sdkerrors.SessionRequiredError{Msg: detailMsg}
			case authv1.AuthErrorCode_AUTH_TIMESTAMP_SKEW:
				return &sdkerrors.TimestampSkewError{Msg: detailMsg, Status: connectHTTPStatus(connectErr.Code())}
			}
			if apiKeyRefusalCodes[authDetail.GetCode()] {
				return &sdkerrors.AuthError{Msg: detailMsg, Code: codeName, Status: connectHTTPStatus(connectErr.Code())}
			}
			return &sdkerrors.APIError{Msg: detailMsg, Code: codeName}
		}
		if rl, ok := value.(*ratelimitv1.RateLimitDetail); ok {
			return rateLimitError(msg, rateLimitDetailFromProto(rl), headerRetry)
		}
		if orderDetail, ok := value.(*orderv1.ErrorDetail); ok {
			rl := rateLimitDetailFromProto(orderDetail.GetRateLimit())
			if rl != nil || orderDetail.GetCode() == orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED {
				return rateLimitError(msg, rl, headerRetry)
			}
			code := orderDetail.GetCode().String()
			if len(orderDetail.GetViolations()) > 0 {
				violations := make([]sdkerrors.FieldViolation, 0, len(orderDetail.GetViolations()))
				for _, violation := range orderDetail.GetViolations() {
					violations = append(violations, sdkerrors.FieldViolation{
						FieldPath: violation.GetFieldPath(),
						RuleID:    violation.GetRuleId(),
						Message:   violation.GetMessage(),
					})
				}
				return &sdkerrors.ValidationError{
					Msg: msg, Code: code, Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations,
				}
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			apiErr := &sdkerrors.APIError{Msg: msg, Code: code}
			if orderDetail.ExistingOrderId != nil {
				apiErr.ExistingOrderID = codecs.FormatUint64ID(orderDetail.GetExistingOrderId())
			}
			return apiErr
		}
		if requestViolations, ok := value.(*validate.Violations); ok && len(requestViolations.GetViolations()) > 0 {
			violations := make([]sdkerrors.FieldViolation, 0, len(requestViolations.GetViolations()))
			for _, violation := range requestViolations.GetViolations() {
				violations = append(violations, sdkerrors.FieldViolation{
					FieldPath: protovalidateFieldPath(violation.GetField()),
					RuleID:    violation.GetRuleId(),
					Message:   violation.GetMessage(),
				})
			}
			return &sdkerrors.ValidationError{
				Msg: msg, Code: connectErr.Code().String(), Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations,
			}
		}
		if withdrawDetail, ok := value.(*withdrawv1.ErrorDetail); ok {
			code := withdrawDetail.GetCode().String()
			if code == withdrawv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED.String() {
				return rateLimitError(msg, nil, headerRetry)
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
		if transferDetail, ok := value.(*transferv1.ErrorDetail); ok {
			code := transferDetail.GetCode().String()
			if code == transferv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED.String() {
				return rateLimitError(msg, nil, headerRetry)
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
	}
	if useragent.IsCloudflareBrowserBan(msg) {
		return &sdkerrors.TransportError{Msg: useragent.Cloudflare1010Message()}
	}
	switch connectErr.Code() {
	case connect.CodeUnauthenticated:
		return &sdkerrors.AuthError{Msg: msg, Code: "unauthenticated", Status: 401}
	case connect.CodePermissionDenied:
		return &sdkerrors.AuthError{Msg: msg, Code: "permission_denied", Status: 403}
	case connect.CodeUnavailable, connect.CodeInternal:
		return &sdkerrors.ServerError{Msg: msg}
	case connect.CodeResourceExhausted:
		return rateLimitError(msg, nil, headerRetry)
	case connect.CodeCanceled, connect.CodeDeadlineExceeded:
		return &sdkerrors.TransportError{Msg: msg}
	case connect.CodeInvalidArgument, connect.CodeFailedPrecondition, connect.CodeOutOfRange:
		return &sdkerrors.ValidationError{Msg: msg, Code: connectErr.Code().String()}
	case connect.CodeUnimplemented:
		if _, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(msg))]; ok {
			return &sdkerrors.RouteNotFoundError{}
		}
		return &sdkerrors.NotImplementedError{Msg: msg}
	case connect.CodeUnknown:
		// Connect reports a non-Connect HTTP error body by its status line
		// and maps 501 to unknown.
		if strings.HasPrefix(msg, "501 ") {
			return &sdkerrors.NotImplementedError{Msg: msg}
		}
	}
	return &sdkerrors.APIError{Msg: msg, Code: connectErr.Code().String()}
}

// apiKeyRefusalCodes are the auth.v1 reasons an API key's credentials were
// refused, other than AUTH_TIMESTAMP_SKEW (a TimestampSkewError). They map to
// *AuthError, as the bare unauthenticated answer that preceded them did.
var apiKeyRefusalCodes = map[authv1.AuthErrorCode]bool{
	authv1.AuthErrorCode_AUTH_SIGNATURE_INVALID:    true,
	authv1.AuthErrorCode_AUTH_MISSING_HEADERS:      true,
	authv1.AuthErrorCode_AUTH_KEY_EXPIRED:          true,
	authv1.AuthErrorCode_AUTH_KEY_DISABLED:         true,
	authv1.AuthErrorCode_AUTH_IP_NOT_ALLOWED:       true,
	authv1.AuthErrorCode_AUTH_API_KEY_NOT_ACCEPTED: true,
}

// protovalidateFieldPath renders a buf.validate field path the way order
// ErrorDetail violations name fields: "items[2].limit_gtc.price_ticks".
func protovalidateFieldPath(path *validate.FieldPath) string {
	var b strings.Builder
	for _, element := range path.GetElements() {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(element.GetFieldName())
		switch subscript := element.GetSubscript().(type) {
		case *validate.FieldPathElement_Index:
			fmt.Fprintf(&b, "[%d]", subscript.Index)
		case *validate.FieldPathElement_BoolKey:
			fmt.Fprintf(&b, "[%t]", subscript.BoolKey)
		case *validate.FieldPathElement_IntKey:
			fmt.Fprintf(&b, "[%d]", subscript.IntKey)
		case *validate.FieldPathElement_UintKey:
			fmt.Fprintf(&b, "[%d]", subscript.UintKey)
		case *validate.FieldPathElement_StringKey:
			fmt.Fprintf(&b, "[%q]", subscript.StringKey)
		}
	}
	return b.String()
}

// TimestampSkewCode is the API-key authentication code for a signature
// timestamp outside the server's skew window.
const TimestampSkewCode = "TIMESTAMP_SKEW"

func connectHTTPStatus(code connect.Code) int {
	switch code {
	case connect.CodeUnauthenticated:
		return 401
	case connect.CodePermissionDenied:
		return 403
	case connect.CodeInvalidArgument:
		return 400
	default:
		return 0
	}
}

func isAuthLikeErrorCode(code string) bool {
	return strings.Contains(code, "UNAUTHENTICATED") ||
		strings.Contains(code, "PERMISSION") ||
		strings.Contains(code, "API_KEY") ||
		strings.Contains(code, "WALLET_BINDING") ||
		strings.Contains(code, "POLICY")
}
