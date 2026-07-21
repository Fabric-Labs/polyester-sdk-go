package wire

import (
	"errors"
	"strings"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
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
