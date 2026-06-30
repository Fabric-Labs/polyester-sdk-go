package wire_test

import (
	"testing"

	layoutv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/layout/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func TestMessageFromMap(t *testing.T) {
	msg := &layoutv1.Layout{}
	if err := wire.MessageFromMap(msg, map[string]any{"name": "test-layout"}); err != nil {
		t.Fatal(err)
	}
	if msg.GetName() != "test-layout" {
		t.Fatalf("name=%q", msg.GetName())
	}
}
