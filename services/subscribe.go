package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// SubscribePublicProto subscribes to a public Centrifugo protobuf channel.
func SubscribePublicProto[T any](ctx context.Context, rt RealtimeClient, channel string, decode func([]byte) (T, error)) (*realtime.Subscription[T], error) {
	if err := requireRealtime(rt); err != nil {
		return nil, err
	}
	return realtime.SubscribeProto(ctx, rt, channel, decode)
}

// SubscribePublicProtoBatch subscribes to a public Centrifugo protobuf
// channel whose publications batch events, emitting each event in order.
func SubscribePublicProtoBatch[T any](ctx context.Context, rt RealtimeClient, channel string, decode func([]byte) ([]T, error)) (*realtime.Subscription[T], error) {
	if err := requireRealtime(rt); err != nil {
		return nil, err
	}
	return realtime.SubscribeProtoBatch(ctx, rt, channel, decode)
}

// SubscribeAccountProto subscribes to a private account-scoped channel.
func SubscribeAccountProto[T any](ctx context.Context, rt RealtimeClient, channelTemplate string, accountID any, defaultAccountID *string, decode func([]byte) (T, error)) (*realtime.Subscription[T], error) {
	channel, err := accountChannel(rt, channelTemplate, accountID, defaultAccountID)
	if err != nil {
		return nil, err
	}
	return realtime.SubscribeProto(ctx, rt, channel, decode)
}

// SubscribeAccountProtoBatch subscribes to a private account-scoped channel
// whose publications batch events, emitting each event in order.
func SubscribeAccountProtoBatch[T any](ctx context.Context, rt RealtimeClient, channelTemplate string, accountID any, defaultAccountID *string, decode func([]byte) ([]T, error)) (*realtime.Subscription[T], error) {
	channel, err := accountChannel(rt, channelTemplate, accountID, defaultAccountID)
	if err != nil {
		return nil, err
	}
	return realtime.SubscribeProtoBatch(ctx, rt, channel, decode)
}

func accountChannel(rt RealtimeClient, channelTemplate string, accountID any, defaultAccountID *string) (string, error) {
	if err := requireRealtime(rt); err != nil {
		return "", err
	}
	resolved, err := ResolveAccountID(accountID, defaultAccountID)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(channelTemplate, "{account_id}", resolved), nil
}
