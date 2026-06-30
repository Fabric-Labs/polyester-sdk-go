package wire

import (
	"errors"
	"strings"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

var routeNotFoundMessages = map[string]struct{}{
	"not found":          {},
	"404 page not found": {},
	"404 not found":      {},
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
	code := connectErr.Code()
	msg := connectErr.Message()
	switch code {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied:
		return &sdkerrors.AuthError{Msg: msg}
	case connect.CodeUnavailable, connect.CodeInternal:
		return &sdkerrors.ServerError{Msg: msg}
	case connect.CodeDeadlineExceeded:
		return &sdkerrors.TransportError{Msg: msg}
	case connect.CodeUnimplemented:
		if _, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(msg))]; ok {
			return &sdkerrors.RouteNotFoundError{}
		}
	}
	return &sdkerrors.APIError{Msg: msg, Code: code.String()}
}
