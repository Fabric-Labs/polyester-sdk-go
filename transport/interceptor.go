package transport

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"google.golang.org/protobuf/proto"
)

// APIKeyInterceptor signs unary Connect requests.
type APIKeyInterceptor struct {
	Credentials *auth.Credentials
	BaseURL     string
}

// NewAPIKeyInterceptor constructs an API-key signing interceptor.
func NewAPIKeyInterceptor(creds *auth.Credentials, baseURL string) *APIKeyInterceptor {
	return &APIKeyInterceptor{
		Credentials: creds,
		BaseURL:     strings.TrimRight(baseURL, "/"),
	}
}

// WrapUnary implements connect.Interceptor.
func (i *APIKeyInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		msg, ok := req.Any().(proto.Message)
		if !ok {
			return next(ctx, req)
		}
		body, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		method := req.HTTPMethod()
		if method == "" {
			method = http.MethodPost
		}
		signURL := auth.RequestURL(i.BaseURL, req.Spec().Procedure)
		headers := auth.SignRequest(i.Credentials, method, signURL, body, "")
		h := req.Header()
		for k, v := range headers {
			h.Set(k, v)
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a no-op for client streaming.
func (i *APIKeyInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler is a no-op on clients.
func (i *APIKeyInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

var _ connect.Interceptor = (*APIKeyInterceptor)(nil)
