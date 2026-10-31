package decode

import (
	"errors"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSpotConfigFromProtoTypesEveryField(t *testing.T) {
	listing := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Assets: []*marketdatav1.AssetConfig{{
			Asset: "BTC", LedgerId: 2, Name: "Bitcoin", QuantityDisplayDecimals: 6,
			QuantityScale: 8, MarketDataVolumeScale: 4,
		}},
		Pairs: []*marketdatav1.PairConfig{{
			SymbolId: 1, Symbol: "BTC-USDT", BaseAsset: "BTC", QuoteAsset: "USDT",
			TickSize: "0.01", StepSize: "0.0001", MinNotionalQuote: "5", MinQtyBase: "0.0001",
			AllowBuyFeeFromBase: true, BaseQuantityScale: 8, QuoteQuantityScale: 6,
			Marketdata:                   &marketdatav1.PairMarketdataConfig{OrderbookPriceBuckets: []float64{0.01, 0.1}},
			ListingAt:                    timestamppb.New(listing),
			Status:                       marketdatav1.PairStatus_PAIR_STATUS_HALTED,
			DefaultMarketSlippageBpsBuy:  50,
			DefaultMarketSlippageBpsSell: 60,
			MaxClientRefDriftBps:         70,
			ReferencePriceScale:          8,
			MaxLimitQtyBase:              proto.String("100"),
			MaxMarketQtyBase:             proto.String("10.5"),
			MaxOrderNotionalQuote:        proto.String("1000000"),
			MaxMarketNotionalQuote:       proto.String("250000.5"),
			MaxOpenOrdersPerPair:         200,
		}},
		TsSec:              1_700_000_000,
		MaxOpenOrdersTotal: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TsSec != 1_700_000_000 || got.MaxOpenOrdersTotal != 1000 || len(got.Assets) != 1 || len(got.Pairs) != 1 {
		t.Fatalf("config=%+v", got)
	}
	asset := got.Assets[0]
	if asset.Asset != "BTC" || asset.LedgerID != 2 || asset.Name != "Bitcoin" || asset.QuantityDisplayDecimals != 6 ||
		asset.QuantityScale != 8 || asset.MarketDataVolumeScale != 4 {
		t.Fatalf("asset=%+v", asset)
	}
	pair := got.Pairs[0]
	if pair.SymbolID != 1 || pair.Symbol != "BTC-USDT" || pair.BaseAsset != "BTC" || pair.QuoteAsset != "USDT" ||
		pair.TickSize != "0.01" || pair.StepSize != "0.0001" || pair.MinNotionalQuote != "5" || pair.MinQtyBase != "0.0001" ||
		!pair.AllowBuyFeeFromBase || pair.BaseQuantityScale != 8 || pair.QuoteQuantityScale != 6 || pair.ReferencePriceScale != 8 ||
		pair.Status != "halted" || pair.DefaultMarketSlippageBpsBuy != 50 || pair.DefaultMarketSlippageBpsSell != 60 ||
		pair.MaxClientRefDriftBps != 70 || len(pair.OrderbookPriceBuckets) != 2 || pair.OrderbookPriceBuckets[1] != 0.1 {
		t.Fatalf("pair=%+v", pair)
	}
	if pair.ListingAt == nil || !pair.ListingAt.Equal(listing) || pair.DelistingAt != nil {
		t.Fatalf("listing=%v delisting=%v", pair.ListingAt, pair.DelistingAt)
	}
	if pair.MaxLimitQtyBase != "100" || pair.MaxMarketQtyBase != "10.5" || pair.MaxOrderNotionalQuote != "1000000" ||
		pair.MaxMarketNotionalQuote != "250000.5" || pair.MaxOpenOrdersPerPair != 200 {
		t.Fatalf("limits=%+v", pair)
	}
	// Catalog hydration still reads the proto-JSON payload.
	if pairs, _ := got.Raw["pairs"].([]any); len(pairs) != 1 {
		t.Fatalf("raw=%#v", got.Raw)
	}
}

func TestSpotConfigFromProtoLeavesAbsentLimitsUnset(t *testing.T) {
	got, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{SymbolId: 1, Symbol: "BTC-USDT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pair := got.Pairs[0]
	if pair.MaxLimitQtyBase != "" || pair.MaxMarketQtyBase != "" || pair.MaxOrderNotionalQuote != "" ||
		pair.MaxMarketNotionalQuote != "" || pair.MaxOpenOrdersPerPair != 0 || got.MaxOpenOrdersTotal != 0 || pair.Status != "unknown" {
		t.Fatalf("pair=%+v total=%d", pair, got.MaxOpenOrdersTotal)
	}
}

func TestSpotConfigFromProtoRejectsMalformedLimits(t *testing.T) {
	for name, pair := range map[string]*marketdatav1.PairConfig{
		"limit qty":                {SymbolId: 1, Symbol: "BTC-USDT", MaxLimitQtyBase: proto.String("-1")},
		"market qty":               {SymbolId: 1, Symbol: "BTC-USDT", MaxMarketQtyBase: proto.String("1e3")},
		"notional":                 {SymbolId: 1, Symbol: "BTC-USDT", MaxOrderNotionalQuote: proto.String("")},
		"trailing dp":              {SymbolId: 1, Symbol: "BTC-USDT", MaxLimitQtyBase: proto.String("1.")},
		"market notional empty":    {SymbolId: 1, Symbol: "BTC-USDT", MaxMarketNotionalQuote: proto.String("")},
		"market notional negative": {SymbolId: 1, Symbol: "BTC-USDT", MaxMarketNotionalQuote: proto.String("-5")},
	} {
		_, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{Pairs: []*marketdatav1.PairConfig{pair}})
		var transportErr *sdkerrors.TransportError
		if !errors.As(err, &transportErr) {
			t.Fatalf("%s: want TransportError, got %T %v", name, err, err)
		}
	}
}

func TestSpotConfigFromProtoReturnsEncodingErrors(t *testing.T) {
	// Proto-JSON rejects invalid UTF-8; the config must fail rather than
	// decode as an empty catalog.
	_, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{SymbolId: 1, Symbol: "BTC-\xff"}},
	})
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("want TransportError, got %T %v", err, err)
	}
}
