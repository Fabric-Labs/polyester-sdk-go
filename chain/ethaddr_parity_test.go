package chain

import (
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/environment"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/ethaddr"
	"github.com/ethereum/go-ethereum/common"
)

// The environment package validates and checksums addresses with ethaddr so
// that it links no go-ethereum. These tests hold ethaddr to go-ethereum's
// common.IsHexAddress and common.HexToAddress(s).Hex(), which chain uses.

func assertEthaddrMatchesGoEthereum(t *testing.T, s string) {
	t.Helper()
	wantValid := common.IsHexAddress(s)
	if got := ethaddr.IsHex(s); got != wantValid {
		t.Fatalf("ethaddr.IsHex(%q) = %v, go-ethereum IsHexAddress = %v", s, got, wantValid)
	}
	got, ok := ethaddr.Checksum(s)
	if ok != wantValid {
		t.Fatalf("ethaddr.Checksum(%q) ok = %v, go-ethereum IsHexAddress = %v", s, ok, wantValid)
	}
	if !wantValid {
		return
	}
	if want := common.HexToAddress(s).Hex(); got != want {
		t.Fatalf("ethaddr.Checksum(%q) = %q, go-ethereum = %q", s, got, want)
	}
}

func ethaddrParitySeeds() []string {
	seeds := []string{
		"", "0x", "0X", "0x0", "zz",
		"0x0000000000000000000000000000000000000000",
		"0xffffffffffffffffffffffffffffffffffffffff",
		"0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0X5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED",
		"5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAe",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed0",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeg",
		" 0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0x0x5aAeb6053F3E94C9b9A09f33669435E7Ef1Be",
	}
	for _, env := range []environment.PolyesterEnvironment{
		environment.PolyesterDevnetEnvironment,
		environment.PolyesterTestnetEnvironment,
	} {
		seeds = append(seeds,
			env.AccountAbstraction.EntryPoint.Address,
			env.AccountAbstraction.Safe.SafeModuleSetupAddress,
			env.AccountAbstraction.Safe.Safe4337ModuleAddress,
			env.AccountAbstraction.Safe.SafeProxyFactoryAddress,
			env.AccountAbstraction.Safe.SafeSingletonAddress,
			env.AccountAbstraction.Safe.MultiSendAddress,
			env.AccountAbstraction.Safe.MultiSendCallOnlyAddress,
			env.Contracts.TradingGatewayAddress,
			env.Contracts.FundingAccountAddress,
			env.Contracts.GuardRegistryAddress,
			env.Contracts.ZipperEndpointAddress,
		)
	}
	return seeds
}

func TestEthaddrMatchesGoEthereum(t *testing.T) {
	for _, s := range ethaddrParitySeeds() {
		assertEthaddrMatchesGoEthereum(t, s)
	}
	rng := rand.New(rand.NewSource(55))
	var raw [20]byte
	for i := 0; i < 5000; i++ {
		rng.Read(raw[:])
		body := hex.EncodeToString(raw[:])
		assertEthaddrMatchesGoEthereum(t, "0x"+body)
		assertEthaddrMatchesGoEthereum(t, "0X"+strings.ToUpper(body))
		assertEthaddrMatchesGoEthereum(t, body)
	}
}

func FuzzEthaddrMatchesGoEthereum(f *testing.F) {
	for _, s := range ethaddrParitySeeds() {
		f.Add(s)
	}
	f.Fuzz(assertEthaddrMatchesGoEthereum)
}
