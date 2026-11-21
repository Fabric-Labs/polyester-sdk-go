package decode

import (
	"testing"
	"time"

	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"google.golang.org/protobuf/proto"
)

// A market overview row carries the pair's live price band, decoded like the
// band GetPriceBand returns; a row without one has no band known.
func TestMarketOverviewEntryReadsLivePriceBand(t *testing.T) {
	asOf := uint64(time.Date(2026, 11, 19, 10, 0, 0, 500_000_000, time.UTC).UnixNano())
	row := topOfBookRow()
	row.PriceBand = &marketdatav1.LivePriceBand{
		SymbolId:             1,
		State:                marketdatav1.PriceBandState_ACTIVE,
		ReferencePriceTicks:  proto.Int64(61_000_250_000_000),
		ReferenceSource:      marketdatav1.PriceBandReferenceSource_LAST_TRADE,
		MaxBuyPriceTicks:     proto.Int64(64_050_250_000_000),
		MinSellPriceTicks:    proto.Int64(57_950_250_000_000),
		UpdatedTsNs:          asOf - 1,
		ReanchorEligibleTsNs: proto.Uint64(asOf + 1),
		Parameters:           &marketdatav1.PriceBand{LowerBps: 500, UpperBps: 500, ReferenceWindowMs: 1000},
		Epoch:                3,
		UpdateId:             41,
		AsOfTsNs:             asOf,
		ValidUntilTsNs:       asOf + 5_000_000_000,
	}
	entry := MarketOverviewEntryFromProto(row, topOfBookCatalogs(t))
	band := entry.PriceBand
	if band == nil {
		t.Fatal("price band dropped")
	}
	if band.State != "active" || band.ReferenceSource != "last_trade" || band.Epoch != 3 || band.UpdateID != 41 ||
		band.MaxBuyPrice == nil || band.MaxBuyPrice.Ticks() != 64_050_250_000_000 || band.MinSellPrice.Ticks() != 57_950_250_000_000 ||
		band.ReferencePrice.Ticks() != 61_000_250_000_000 || band.ReferencePrice.Format() != "61000.25" || band.MinBuyPrice != nil || band.MaxSellPrice != nil ||
		band.ReanchorEligibleTsNs == nil || *band.ReanchorEligibleTsNs != asOf+1 ||
		band.Parameters == nil || band.Parameters.LowerBps != 500 || band.Parameters.ReferenceWindowMs != 1000 {
		t.Fatalf("band = %+v", band)
	}
	at := time.Unix(0, int64(asOf))
	if !band.FreshAt(at) || !band.FreshAt(at.Add(5*time.Second)) || band.FreshAt(at.Add(5*time.Second+time.Nanosecond)) {
		t.Fatal("band freshness does not follow valid_until")
	}

	unknown := LivePriceBandFromProto(&marketdatav1.LivePriceBand{State: marketdatav1.PriceBandState_UNKNOWN, ValidUntilTsNs: asOf}, "")
	if unknown.State != "unknown" || unknown.FreshAt(at) {
		t.Fatalf("unknown band = %+v", unknown)
	}
	without := MarketOverviewEntryFromProto(&overviewpb.MarketOverview{SymbolId: 1}, nil)
	if without.PriceBand != nil || without.PriceBand.FreshAt(at) {
		t.Fatalf("row without a band = %+v", without.PriceBand)
	}
}
