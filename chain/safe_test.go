package chain

import (
	"math/big"
	"strings"
	"testing"
)

// Golden vectors from polyester-sdk-typescript predict-safe-address (key = 0x01).
const (
	testOwner = "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"
	testSalt0 = "0xA244Ed1dc6B46C75F37E0119054fFa45E76c9B6f"
	testSalt7 = "0x4AEdcc90537f9fb3828E6b431E5A16Cdc473D6f0"
)

func TestPredictSafeAddressSaltZero(t *testing.T) {
	got, err := PredictSafeAddress(testOwner, big.NewInt(0), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got, testSalt0) {
		t.Fatalf("salt 0: got %s want %s", got, testSalt0)
	}
}

func TestPredictSafeAddressSaltSeven(t *testing.T) {
	got, err := PredictSafeAddress(testOwner, big.NewInt(7), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got, testSalt7) {
		t.Fatalf("salt 7: got %s want %s", got, testSalt7)
	}
}

func TestPredictSafeAddressWithDataIncludesFactoryCalldata(t *testing.T) {
	predicted, err := PredictSafeAddressWithData(PredictSafeAddressOptions{
		Owners:    []string{testOwner},
		SaltNonce: big.NewInt(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(predicted.Address, testSalt0) {
		t.Fatalf("address: got %s want %s", predicted.Address, testSalt0)
	}
	if hex := strings.ToLower(encodeHex(predicted.Initializer)); !strings.HasPrefix(hex, "b63e800d") {
		t.Fatalf("initializer should start with setup selector, got %s", hex[:min(16, len(hex))])
	}
	if hex := strings.ToLower(encodeHex(predicted.FactoryCalldata)); !strings.HasPrefix(hex, "1688f0b9") {
		t.Fatalf("factory calldata should start with createProxy selector, got %s", hex[:min(16, len(hex))])
	}
}

func encodeHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}
