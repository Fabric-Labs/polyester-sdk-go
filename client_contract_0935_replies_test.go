package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeZipperConfig struct {
	chainzipperv1connect.UnimplementedZipperServiceHandler
	config *zipperv1.GetDepositWithdrawConfigResponse
}

func (f *fakeZipperConfig) GetDepositWithdrawConfig(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	return connect.NewResponse(f.config), nil
}

type fakeMarketStats struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	reply *marketdatav1.ListAssetInfoResponse
}

func (f *fakeMarketStats) ListAssetInfo(context.Context, *connect.Request[marketdatav1.ListAssetInfoRequest]) (*connect.Response[marketdatav1.ListAssetInfoResponse], error) {
	return connect.NewResponse(f.reply), nil
}

var observedAt = time.Date(2026, 11, 15, 12, 0, 0, 500_000_000, time.UTC)

func supplyConfig(status zipperv1.SupplyStatus, observed *timestamppb.Timestamp) *zipperv1.GetDepositWithdrawConfigResponse {
	return &zipperv1.GetDepositWithdrawConfigResponse{
		Chains: []*zipperv1.ChainConfig{{ChainId: 2, Code: "chain", Name: "Chain"}},
		Assets: []*zipperv1.AssetConfig{{Asset: "USDT", LedgerId: 1, QuantityScale: 6, Variants: []*zipperv1.AssetChainVariant{{
			ZippedAssetId: 3, ChainId: 2, SupplyQ: 1_250_000_500_000, SupplyStatus: status, SupplyObservedAt: observed,
		}}}},
	}
}

// A known supply (AVAILABLE or STALE) comes with the time it was read; one
// without it breaks the contract, unary and on the stream alike. A missing
// supply names no time.
func TestKnownSupplyWithoutItsObservationTimeFailsClosed(t *testing.T) {
	fake := &fakeZipperConfig{}
	client := newContract0935Client(t, func(mux *http.ServeMux) {
		path, handler := chainzipperv1connect.NewZipperServiceHandler(fake)
		mux.Handle(path, handler)
	})
	for _, status := range []zipperv1.SupplyStatus{zipperv1.SupplyStatus_AVAILABLE, zipperv1.SupplyStatus_STALE} {
		fake.config = supplyConfig(status, nil)
		var contract *sdkerrors.ResponseContractError
		if _, err := client.Zipper.GetDepositWithdrawConfig(context.Background()); !stderrors.As(err, &contract) {
			t.Fatalf("%s without supply_observed_at: err = %v, want a ResponseContractError", status, err)
		}
		fake.config = supplyConfig(status, timestamppb.New(observedAt))
		cfg, err := client.Zipper.GetDepositWithdrawConfig(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if v := cfg.Assets[0].Variants[0]; v.Supply != "1250000.5" || v.SupplyObservedAt == nil || !v.SupplyObservedAt.Equal(observedAt) {
			t.Fatalf("%s variant = %+v", status, v)
		}
	}
	fake.config = supplyConfig(zipperv1.SupplyStatus_MISSING, nil)
	if cfg, err := client.Zipper.GetDepositWithdrawConfig(context.Background()); err != nil || cfg.Assets[0].Variants[0].Supply != "" {
		t.Fatalf("missing supply = %+v, %v", cfg, err)
	}

	scale := func(uint32) (int, bool) { return 6, true }
	for _, status := range []zipperv1.SupplyStatus{zipperv1.SupplyStatus_AVAILABLE, zipperv1.SupplyStatus_STALE} {
		payload, err := proto.Marshal(&zipperv1.ZippedAssetSupplyBatch{Updates: []*zipperv1.ZippedAssetSupplyUpdate{{ZippedAssetId: 3, SupplyQ: 7, Status: status}}})
		if err != nil {
			t.Fatal(err)
		}
		var contract *sdkerrors.ResponseContractError
		if _, err := decode.ZippedAssetSupplyBatchFromBytes(payload, scale); !stderrors.As(err, &contract) {
			t.Fatalf("stream %s without observed_at: err = %v, want a ResponseContractError", status, err)
		}
	}
}

// A market datapoint that is present names its value, source and time; one
// missing any of them breaks the contract. An absent datapoint is unknown.
func TestAssetMarketValueMissingAPartFailsClosed(t *testing.T) {
	fake := &fakeMarketStats{}
	client := newContract0935Client(t, func(mux *http.ServeMux) {
		path, handler := marketdatav1connect.NewMarketDataServiceHandler(fake)
		mux.Handle(path, handler)
	})
	full := func() *marketdatav1.AssetMarketValue {
		return &marketdatav1.AssetMarketValue{Value: "20113162", Source: marketdatav1.AssetDataSource_COINMARKETCAP, AsOf: timestamppb.New(observedAt)}
	}
	for name, breakValue := range map[string]func(*marketdatav1.AssetMarketValue){
		"no value":  func(v *marketdatav1.AssetMarketValue) { v.Value = "" },
		"no source": func(v *marketdatav1.AssetMarketValue) { v.Source = marketdatav1.AssetDataSource_SOURCE_UNSPECIFIED },
		"no as_of":  func(v *marketdatav1.AssetMarketValue) { v.AsOf = nil },
	} {
		value := full()
		breakValue(value)
		fake.reply = &marketdatav1.ListAssetInfoResponse{AssetInfo: []*marketdatav1.AssetInfo{{AssetId: 1, MarketStats: &marketdatav1.AssetMarketStats{Rank: value}}}}
		var contract *sdkerrors.ResponseContractError
		if _, err := client.MarketData.ListAssetInfo(context.Background()); !stderrors.As(err, &contract) {
			t.Fatalf("%s: err = %v, want a ResponseContractError", name, err)
		}
	}
	// A presence-only fact names its source and time too.
	for name, flag := range map[string]*marketdatav1.AssetMarketFlag{
		"no source": {AsOf: timestamppb.New(observedAt)},
		"no as_of":  {Source: marketdatav1.AssetDataSource_COINMARKETCAP},
	} {
		fake.reply = &marketdatav1.ListAssetInfoResponse{AssetInfo: []*marketdatav1.AssetInfo{{AssetId: 1, MarketStats: &marketdatav1.AssetMarketStats{MaxSupplyUnlimited: flag}}}}
		var contract *sdkerrors.ResponseContractError
		if _, err := client.MarketData.ListAssetInfo(context.Background()); !stderrors.As(err, &contract) {
			t.Fatalf("flag %s: err = %v, want a ResponseContractError", name, err)
		}
	}
	fake.reply = &marketdatav1.ListAssetInfoResponse{AssetInfo: []*marketdatav1.AssetInfo{{AssetId: 1, MarketStats: &marketdatav1.AssetMarketStats{Rank: full()}}}}
	list, err := client.MarketData.ListAssetInfo(context.Background())
	if err != nil || list[0].MarketStats.Rank == nil || list[0].MarketStats.Rank.Value != "20113162" || list[0].MarketStats.TotalSupply != nil {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
