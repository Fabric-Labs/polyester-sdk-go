package wire

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
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
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return &sdkerrors.TransportError{Msg: err.Error()}
	}
	headerRetry := retryAfterSeconds(connectErr)
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
				metadata := make(map[string]string, len(orderDetail.GetViolations())*2)
				for i, violation := range orderDetail.GetViolations() {
					prefix := fmt.Sprintf("violation.%d.", i)
					metadata[prefix+"field_path"] = violation.GetFieldPath()
					metadata[prefix+"rule_id"] = violation.GetRuleId()
					metadata[prefix+"message"] = violation.GetMessage()
				}
				return &sdkerrors.ValidationError{Msg: msg, Code: code, Metadata: metadata}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
	}
	if useragent.IsCloudflareBrowserBan(msg) {
		return &sdkerrors.TransportError{Msg: useragent.Cloudflare1010Message()}
	}
	switch connectErr.Code() {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied:
		return &sdkerrors.AuthError{Msg: msg}
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
	}
	return &sdkerrors.APIError{Msg: msg, Code: connectErr.Code().String()}
}
