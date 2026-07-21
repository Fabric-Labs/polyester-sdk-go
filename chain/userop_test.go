package chain

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestExecuteUserOpSelector(t *testing.T) {
	call := ChainCall{
		To:    "0xD3fecf5D39131e23b6B0f872cA0a21c8A5a30932",
		Data:  []byte{0x12, 0x34},
		Value: big.NewInt(0),
	}
	encoded, err := EncodeExecuteUserOpCallData(call)
	if err != nil {
		t.Fatal(err)
	}
	expected := crypto.Keccak256([]byte("executeUserOpWithErrorString(address,uint256,bytes,uint8)"))[:4]
	if len(encoded) < 4 {
		t.Fatalf("encoded too short: %d", len(encoded))
	}
	for i := 0; i < 4; i++ {
		if encoded[i] != expected[i] {
			t.Fatalf("selector mismatch: got %x want %x", encoded[:4], expected)
		}
	}
	if encodeHex(encoded[:4]) != executeUserOpSelectorHex {
		t.Fatalf("selector hex: got %s want %s", encodeHex(encoded[:4]), executeUserOpSelectorHex)
	}
}

func TestGasBufferAppliesMinimum(t *testing.T) {
	got := AddUserOperationGasBuffer(big.NewInt(100))
	if got.Cmp(big.NewInt(50_100)) != 0 {
		t.Fatalf("small gas: got %s want 50100", got)
	}
	got = AddUserOperationGasBuffer(big.NewInt(1_000_000))
	if got.Cmp(big.NewInt(1_200_000)) != 0 {
		t.Fatalf("large gas: got %s want 1200000", got)
	}
}

func TestPackPaymasterAndDataEmptyWithoutPaymaster(t *testing.T) {
	got := PackPaymasterAndData("", nil, nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %x", got)
	}
}
