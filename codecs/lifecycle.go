package codecs

import (
	"strings"

	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
)

// TxLookupKindFromLabel maps a tx lookup kind label to the proto enum.
func TxLookupKindFromLabel(value string) lifecyclev1.TxLookupKind {
	kindName := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	if kindName == "" {
		return lifecyclev1.TxLookupKind_TX_ANY
	}
	if !strings.HasPrefix(kindName, "TX_") {
		kindName = "TX_" + kindName
	}
	if v, ok := lifecyclev1.TxLookupKind_value[kindName]; ok && v != 0 {
		return lifecyclev1.TxLookupKind(v)
	}
	return lifecyclev1.TxLookupKind_TX_ANY
}
