package wire

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
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

// MapConnectError converts Connect errors into SDK error types.
func MapConnectError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return &sdkerrors.TransportError{Msg: err.Error()}
	}
	for _, detail := range connectErr.Details() {
		msg, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if authDetail, ok := msg.(*authv1.AuthErrorDetail); ok {
			codeName := authDetail.GetCode().String()
			detailMsg := authDetail.GetMessage()
			if detailMsg == "" {
				detailMsg = connectErr.Message()
			}
			return &sdkerrors.APIError{Msg: detailMsg, Code: codeName}
		}
	}
	code := connectErr.Code()
	msg := connectErr.Message()
	if strings.TrimSpace(msg) == "" {
		msg = emptyErrorMessage
	}
	if useragent.IsCloudflareBrowserBan(msg) {
		return &sdkerrors.TransportError{Msg: useragent.Cloudflare1010Message()}
	}
	switch code {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied:
		return &sdkerrors.AuthError{Msg: msg}
	case connect.CodeUnavailable, connect.CodeInternal:
		return &sdkerrors.ServerError{Msg: msg}
	case connect.CodeResourceExhausted:
		return &sdkerrors.RateLimitError{
			Msg:        msg,
			RetryAfter: retryAfterSeconds(connectErr),
		}
	case connect.CodeDeadlineExceeded:
		return &sdkerrors.TransportError{Msg: msg}
	case connect.CodeUnimplemented:
		if _, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(msg))]; ok {
			return &sdkerrors.RouteNotFoundError{}
		}
	}
	return &sdkerrors.APIError{Msg: msg, Code: code.String()}
}
