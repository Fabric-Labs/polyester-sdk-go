package decode

import (
	"testing"

	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Contract guard_signer.proto, GetGuardSignerStatusResponse.status: the
// signer wallet, the on-chain signer address, initialized, nonce (a decimal
// string) and nonce_space (uint64). GetStatus keeps all of them.
func TestGuardSignerStatusKeepsOnchainSignerAndNonce(t *testing.T) {
	got := StatusFromProto(&guardv1.GetGuardSignerStatusResponse{Status: &guardv1.GuardSignerStatus{
		SignerAddress:        "0x5555555555555555555555555555555555555555",
		OnchainSignerAddress: "0x6666666666666666666666666666666666666666",
		Nonce:                "42",
		NonceSpace:           7,
	}})
	want := models.GuardSignerStatus{
		SignerWallet:         "0x5555555555555555555555555555555555555555",
		Status:               "uninitialized",
		OnchainSignerAddress: "0x6666666666666666666666666666666666666666",
		Nonce:                "42",
		NonceSpace:           7,
	}
	if got == nil || *got != want {
		t.Fatalf("StatusFromProto = %+v, want %+v", got, want)
	}
}
