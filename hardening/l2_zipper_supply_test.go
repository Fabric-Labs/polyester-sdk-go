package hardening_test

import (
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

// supplyCase is one variant's raw supply_q and the decimal the SDK must
// report for it at its asset's quantity scale.
type supplyCase struct {
	zippedAssetID uint32
	supplyQ       uint64
	want          string
	status        zipperv1.SupplyStatus
}

// GetDepositWithdrawConfig keeps each variant's supply, supply_q at its
// parent asset's quantity_scale, in the typed result and in the catalog the
// client hydrates at construction, in both wire formats: zero and positive
// amounts at scale 0 and scale 8.
func TestL2DepositWithdrawConfigKeepsVariantSupply(t *testing.T) {
	// A supply is known only while its status is AVAILABLE or STALE; a
	// MISSING or unspecified one reads no supply, never "0", whatever
	// supply_q says.
	scale0 := []supplyCase{
		{zippedAssetID: 11, supplyQ: 3582802, want: "3582802", status: zipperv1.SupplyStatus_AVAILABLE},
		{zippedAssetID: 12, supplyQ: 0, want: "0", status: zipperv1.SupplyStatus_STALE},
		{zippedAssetID: 13, supplyQ: 0, want: "", status: zipperv1.SupplyStatus_MISSING},
		{zippedAssetID: 14, supplyQ: 7, want: "", status: zipperv1.SupplyStatus_SUPPLY_UNSPECIFIED},
	}
	scale8 := []supplyCase{
		{zippedAssetID: 21, supplyQ: 9708173223, want: "97.08173223", status: zipperv1.SupplyStatus_AVAILABLE},
		{zippedAssetID: 22, supplyQ: 0, want: "0", status: zipperv1.SupplyStatus_AVAILABLE},
	}
	variants := func(cases []supplyCase) []*zipperv1.AssetChainVariant {
		out := make([]*zipperv1.AssetChainVariant, 0, len(cases))
		for _, c := range cases {
			out = append(out, &zipperv1.AssetChainVariant{
				ZippedAssetId:     c.zippedAssetID,
				ChainId:           c.zippedAssetID,
				NetworkFee:        "0.0001",
				DepositMinAmount:  "0.001",
				WithdrawMinAmount: "0.002",
				SupplyQ:           c.supplyQ,
				SupplyStatus:      c.status,
				SupplyObservedAt:  supplyObservedAt(c.status),
			})
		}
		return out
	}
	var chains []*zipperv1.ChainConfig
	for _, c := range append(append([]supplyCase{}, scale0...), scale8...) {
		chains = append(chains, &zipperv1.ChainConfig{ChainId: c.zippedAssetID, Code: "chain", Name: "Chain"})
	}
	config := &zipperv1.GetDepositWithdrawConfigResponse{
		Chains: chains,
		Assets: []*zipperv1.AssetConfig{
			{Asset: "WHOLE", LedgerId: 1, QuantityScale: 0, Variants: variants(scale0)},
			{Asset: "BTC", LedgerId: 2, QuantityScale: 8, Variants: variants(scale8)},
		},
	}
	want := map[uint32]string{}
	for _, c := range append(append([]supplyCase{}, scale0...), scale8...) {
		want[c.zippedAssetID] = c.want
	}

	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
				return connect.NewResponse(hydrationSpotConfig()), nil
			}}, zipperHandler{fn: func(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
				return connect.NewResponse(config), nil
			}})
			client, err := polyester.New(polyester.Config{APIURL: srv.URL, WireFormat: wire, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })

			typed, err := client.Zipper.GetDepositWithdrawConfig(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			seen := 0
			for _, asset := range typed.Assets {
				for _, variant := range asset.Variants {
					seen++
					if variant.Supply != want[variant.ZippedAssetID] {
						t.Errorf("typed %s variant %d supply = %q, want %q", asset.Asset, variant.ZippedAssetID, variant.Supply, want[variant.ZippedAssetID])
					}
					if variant.NetworkFee != "0.0001" || variant.DepositMinAmount != "0.001" || variant.WithdrawMinAmount != "0.002" {
						t.Errorf("typed %s variant %d lost its fee or minimums: %+v", asset.Asset, variant.ZippedAssetID, variant)
					}
				}
			}
			if seen != len(want) {
				t.Fatalf("typed result has %d variants, want %d", seen, len(want))
			}

			// Spot readiness does not imply the funding config: wait for it.
			if err := client.WaitForFundingCatalog(context.Background()); err != nil {
				t.Fatal(err)
			}
			catalog := client.Catalogs.Zipper
			if catalog == nil {
				t.Fatal("zipper catalog was not hydrated")
			}
			seen = 0
			for _, asset := range catalog.Assets {
				for _, chain := range asset.Chains {
					seen++
					if chain.Supply != want[chain.ZippedAssetID] {
						t.Errorf("catalog %s variant %d supply = %q, want %q", asset.Asset, chain.ZippedAssetID, chain.Supply, want[chain.ZippedAssetID])
					}
				}
			}
			if seen != len(want) {
				t.Fatalf("catalog has %d variants, want %d", seen, len(want))
			}
		})
	}
}

// supplyObservedAt is the read time a known supply (AVAILABLE or STALE)
// carries; nil for any other status.
func supplyObservedAt(status zipperv1.SupplyStatus) *timestamppb.Timestamp {
	if status != zipperv1.SupplyStatus_AVAILABLE && status != zipperv1.SupplyStatus_STALE {
		return nil
	}
	return timestamppb.New(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
}
