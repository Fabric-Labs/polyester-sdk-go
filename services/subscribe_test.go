package services

import (
	"context"
	"testing"
)

func TestResolveAccountIDRequiresValue(t *testing.T) {
	_, err := ResolveAccountID(nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSubscribePublicProtoRequiresRealtime(t *testing.T) {
	_, err := SubscribePublicProto(context.TODO(), nil, "public:identity:updates:proto", func([]byte) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil {
		t.Fatal("expected realtime error")
	}
}
