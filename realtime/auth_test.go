package realtime

import (
	"context"
	"errors"
	"net/http"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestContentLengthExceedsLimit(t *testing.T) {
	header := http.Header{}
	header.Set("Content-Length", "65537")
	if !contentLengthExceedsLimit(header, maxTokenResponseBytes) {
		t.Fatal("expected oversized content-length to exceed limit")
	}
	header.Set("Content-Length", "1024")
	if contentLengthExceedsLimit(header, maxTokenResponseBytes) {
		t.Fatal("expected small content-length to pass")
	}
}

func TestUnauthenticatedPrivateSubscribeFailsBeforeDial(t *testing.T) {
	client := NewClient("ws://127.0.0.1:1", "http://127.0.0.1:1", nil, nil, 1)
	_, err := SubscribeProto(context.Background(), client, "private:spot:orders:1:proto", func([]byte) (struct{}, error) {
		return struct{}{}, nil
	})
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %T: %v", err, err)
	}
}
