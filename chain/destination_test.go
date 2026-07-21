package chain_test

import (
	"bytes"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/chain"
)

func TestEncodeWithdrawDestinationCaseSensitive(t *testing.T) {
	address := "Tb1QCaseSensitiveAddress"
	got := chain.EncodeWithdrawDestination(address, true)
	if !bytes.Equal(got, []byte(address)) {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeWithdrawDestinationLowercases(t *testing.T) {
	address := "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"
	got := chain.EncodeWithdrawDestination(address, false)
	want := []byte("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
