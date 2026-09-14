//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
)

func TestDevnetPinsMatchLiveZipperCatalog(t *testing.T) {
	assertPinsMatchCatalog(t, chain.PolyesterDevnetEnvironment)
}

func TestTestnetPinsMatchLiveZipperCatalog(t *testing.T) {
	assertPinsMatchCatalog(t, chain.PolyesterTestnetEnvironment)
}

func assertPinsMatchCatalog(t *testing.T, env chain.PolyesterEnvironment) {
	t.Helper()
	client, err := polyester.New(polyester.Config{
		Environment:     &env,
		HydrateCatalogs: false,
		Timeout:         15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg, err := client.Zipper.GetDepositWithdrawConfig(ctx)
	if err != nil {
		testutil.SoftSkipf(t, "live zipper catalog unavailable: %v", err)
	}
	byName := map[string]string{}
	for _, item := range cfg.Contracts {
		byName[item.Name] = item.Address
	}
	expected := map[string]string{
		"tradingGateway": env.Contracts.TradingGatewayAddress,
		"fundingAccount": env.Contracts.FundingAccountAddress,
		"guardRegistry":  env.Contracts.GuardRegistryAddress,
		"zipperEndpoint": env.Contracts.ZipperEndpointAddress,
		"EntryPoint":     env.AccountAbstraction.EntryPoint.Address,
	}
	for name, address := range expected {
		got, ok := byName[name]
		if !ok {
			t.Fatalf("%s catalog missing %s", env.Name, name)
		}
		if !strings.EqualFold(got, address) {
			t.Fatalf("%s %s: catalog %s != pin %s", env.Name, name, got, address)
		}
	}
}
