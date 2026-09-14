package chain

import "testing"

func TestDevnetPresetMatchesTypeScriptAndZipperExtras(t *testing.T) {
	env := PolyesterDevnetEnvironment
	if env.Name != "polyester-devnet" {
		t.Fatalf("name=%s", env.Name)
	}
	if env.APIURL != "https://api-devnet.polyester.ai" || env.ChainID != 888168 {
		t.Fatalf("devnet urls/chain: %s %d", env.APIURL, env.ChainID)
	}
	if env.Contracts.TradingGatewayAddress != "0xD3fecf5D39131e23b6B0f872cA0a21c8A5a30932" {
		t.Fatalf("trading gateway=%s", env.Contracts.TradingGatewayAddress)
	}
	if env.Contracts.FundingAccountAddress != "0xBfF4F6224BC10f233dDB1E61E770d9832aabC7c4" {
		t.Fatalf("funding=%s", env.Contracts.FundingAccountAddress)
	}
}

func TestTestnetPresetMatchesTypeScriptAndZipperExtras(t *testing.T) {
	env := PolyesterTestnetEnvironment
	if env.Name != "polyester-testnet" || env.ChainID != 888169 {
		t.Fatalf("testnet %s %d", env.Name, env.ChainID)
	}
	if env.APIURL != "https://api-testnet.polyester.com" {
		t.Fatalf("api=%s", env.APIURL)
	}
	if env.Contracts.TradingGatewayAddress != "0x20ef1BCeE69D73Ce1649E688dAA9A7AcF441f0EE" {
		t.Fatalf("trading gateway=%s", env.Contracts.TradingGatewayAddress)
	}
	if env.Contracts.ZipperEndpointAddress != "0xD439270f881b56727EaaB878CE4e80eB08A25BEB" {
		t.Fatalf("zipper=%s", env.Contracts.ZipperEndpointAddress)
	}
}

func TestWithURLsKeepsChainPins(t *testing.T) {
	custom, err := PolyesterTestnetEnvironment.WithURLs(
		"https://mm.internal.example",
		"wss://mm.internal.example",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if custom.APIURL != "https://mm.internal.example" {
		t.Fatalf("api=%s", custom.APIURL)
	}
	if custom.ChainID != PolyesterTestnetEnvironment.ChainID {
		t.Fatalf("chain id changed: %d", custom.ChainID)
	}
	if custom.Contracts != PolyesterTestnetEnvironment.Contracts {
		t.Fatal("contracts changed")
	}
}

func TestCreateRejectsRemotePlaintext(t *testing.T) {
	_, err := PolyesterDevnetEnvironment.WithURLs("http://api.example.test", "", "")
	if err == nil {
		t.Fatal("expected insecure remote http to fail")
	}
}

func TestCreateAllowsLoopbackHTTP(t *testing.T) {
	custom, err := PolyesterDevnetEnvironment.WithURLs("http://127.0.0.1:8080", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if custom.APIURL != "http://127.0.0.1:8080" {
		t.Fatalf("api=%s", custom.APIURL)
	}
}

func TestEnvironmentFromName(t *testing.T) {
	devnet, err := EnvironmentFromName("devnet")
	if err != nil || devnet.ChainID != 888168 {
		t.Fatalf("devnet: %+v %v", devnet, err)
	}
	_, err = EnvironmentFromName("mainnet")
	if err == nil {
		t.Fatal("expected unknown environment")
	}
}
