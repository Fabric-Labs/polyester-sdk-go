package wire_test

import (
	"testing"

	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func TestMessageFromMap(t *testing.T) {
	msg := &typev1.U128{}
	if err := wire.MessageFromMap(msg, map[string]any{"lo": 7}); err != nil {
		t.Fatal(err)
	}
	if msg.GetLo() != 7 {
		t.Fatalf("lo=%d", msg.GetLo())
	}
}
