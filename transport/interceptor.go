package transport

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// APIKeyInterceptor signs unary Connect requests over the exact body bytes the
// configured wire format will transmit (binary protobuf or ProtoJSON).
type APIKeyInterceptor struct {
	Credentials *auth.Credentials
	BaseURL     string
	WireFormat  connectx.WireFormat
}

// NewAPIKeyInterceptor constructs an API-key signing interceptor.
func NewAPIKeyInterceptor(creds *auth.Credentials, baseURL string, wire connectx.WireFormat) *APIKeyInterceptor {
	return &APIKeyInterceptor{
		Credentials: creds,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		WireFormat:  wire,
	}
}

// WrapUnary implements connect.Interceptor.
func (i *APIKeyInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		msg, ok := req.Any().(proto.Message)
		if !ok {
			return next(ctx, req)
		}
		body, err := encodeWireBody(msg, i.WireFormat)
		if err != nil {
			return nil, err
		}
		method := req.HTTPMethod()
		if method == "" {
			method = http.MethodPost
		}
		signURL := auth.RequestURL(i.BaseURL, req.Spec().Procedure)
		headers, err := auth.SignRequest(i.Credentials, method, signURL, body, "")
		if err != nil {
			return nil, err
		}
		h := req.Header()
		for k, v := range headers {
			h.Set(k, v)
		}
		return next(ctx, req)
	}
}

func encodeWireBody(msg proto.Message, wire connectx.WireFormat) ([]byte, error) {
	if wire == connectx.WireJSON {
		// Must match connect.WithProtoJSON() / connect's jsonCodec (default protojson).
		return protojson.MarshalOptions{}.Marshal(msg)
	}
	return proto.Marshal(msg)
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
