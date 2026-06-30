package codecs_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
)

func TestTxLookupKindFromLabel(t *testing.T) {
	if got := codecs.TxLookupKindFromLabel("any"); got != lifecyclev1.TxLookupKind_TX_ANY {
		t.Fatalf("any: got %v", got)
	}
	if got := codecs.TxLookupKindFromLabel("source"); got != lifecyclev1.TxLookupKind_TX_SOURCE {
		t.Fatalf("source: got %v", got)
	}
}
