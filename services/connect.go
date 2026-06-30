package services

import (
	"context"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// UnaryPublic calls a public Connect RPC and decodes the response.
func UnaryPublic[Req any, Resp any, Out any](
	ctx context.Context,
	factory *transport.Factory,
	call func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
	req *Req,
	decode func(*Resp) Out,
) (Out, error) {
	var zero Out
	resp, err := call(ctx, connect.NewRequest(req))
	if err != nil {
		return zero, transport.MapError(err)
	}
	return decode(resp.Msg), nil
}

// UnaryAuth calls an authenticated Connect RPC and decodes the response.
func UnaryAuth[Req any, Resp any, Out any](
	ctx context.Context,
	factory *transport.Factory,
	call func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error),
	req *Req,
	decode func(*Resp) Out,
) (Out, error) {
	var zero Out
	if _, err := factory.RequireCredentials(); err != nil {
		return zero, err
	}
	resp, err := call(ctx, connect.NewRequest(req))
	if err != nil {
		return zero, transport.MapError(err)
	}
	return decode(resp.Msg), nil
}
