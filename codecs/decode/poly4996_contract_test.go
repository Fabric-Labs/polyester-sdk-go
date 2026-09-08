package decode

import (
	"testing"

	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

func ptrInt64(v int64) *int64 { return &v }

func TestLadderDetailsDecodeExecutedFields(t *testing.T) {
	msg := &triggersv1.Trigger{
		TriggerId: 9,
		SymbolId:  1,
		Status:    triggersv1.TriggerStatus_STATUS_RUNNING,
		QtyScaled: 100_000_000,
		Configuration: &triggersv1.Trigger_Ladder{
			Ladder: &triggersv1.LadderTrigger{
				Side:          orderv1.Side_BUY,
				PriceMinTicks: 1,
				PriceMaxTicks: 2,
				Levels:        4,
			},
		},
		RuntimeDetails: &triggersv1.Trigger_LadderState{
			LadderState: &triggersv1.LadderDetails{
				LadderPriceMinTicks: 1,
				LadderPriceMaxTicks: 2,
				LadderLevels:        4,
				ExecutedQtyScaled:   25_000_000,
				ExecutedLevels:      2,
			},
		},
	}
	trigger := TriggerFromProto(msg)
	if trigger.Details == nil || trigger.Details.Case != "ladder" {
		t.Fatalf("details=%+v", trigger.Details)
	}
	if trigger.Details.ExecutedLevels != 2 {
		t.Fatalf("executed_levels=%d", trigger.Details.ExecutedLevels)
	}
	if trigger.Details.ExecutedQty.Scaled() != 25_000_000 {
		t.Fatalf("executed_qty=%+v", trigger.Details.ExecutedQty)
	}
}

func TestCandlePointPreservesQuoteVolume(t *testing.T) {
	candle := CandlePointFromProto(&marketdatav1.CandlePoint{
		TsSec:       1,
		Open:        1_000_000,
		High:        2_000_000,
		Low:         500_000,
		Close:       1_500_000,
		Volume:      10,
		QuoteVolume: "150.25",
		IsClosed:    true,
	}, 1, "1m", 8)
	if candle.QuoteVolume != "150.25" {
		t.Fatalf("quote_volume=%q", candle.QuoteVolume)
	}
	columns, err := CandlesColumnsFromProto(&marketdatav1.GetCandlesColumnsResponse{
		SymbolId:    1,
		Timeframe:   marketdatav1.Timeframe_MIN_1,
		TsSec:       []uint64{1},
		Open:        []int64{1_000_000},
		High:        []int64{2_000_000},
		Low:         []int64{500_000},
		Close:       []int64{1_500_000},
		Volume:      []int64{10},
		QuoteVolume: []string{"150.25"},
	}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if columns.Candles[0].QuoteVolume != "150.25" {
		t.Fatalf("column quote_volume=%q", columns.Candles[0].QuoteVolume)
	}
}

func TestMarketOverviewOptionalVolumeFields(t *testing.T) {
	present := MarketOverviewEntryFromProto(&marketoverviewv1.MarketOverview{
		SymbolId:              1,
		Volume_24HBaseScaled:  ptrInt64(10),
		Volume_24HQuoteScaled: ptrInt64(20),
		Volume_24HUsdScaled:   ptrInt64(30),
	}, nil)
	if present.Volume24HBaseScaled == nil || *present.Volume24HBaseScaled != "10" {
		t.Fatalf("base=%v", present.Volume24HBaseScaled)
	}
	if present.Volume24HQuoteScaled == nil || *present.Volume24HQuoteScaled != "20" {
		t.Fatalf("quote=%v", present.Volume24HQuoteScaled)
	}
	if present.Volume24HUsdScaled == nil || *present.Volume24HUsdScaled != "30" {
		t.Fatalf("usd=%v", present.Volume24HUsdScaled)
	}

	absent := MarketOverviewEntryFromProto(&marketoverviewv1.MarketOverview{SymbolId: 2}, nil)
	if absent.Volume24HBaseScaled != nil || absent.Volume24HQuoteScaled != nil || absent.Volume24HUsdScaled != nil {
		t.Fatalf("absent volumes should stay nil: %+v", absent)
	}
}

func TestSpotVolumeHistoryFromProto(t *testing.T) {
	result := SpotVolumeHistoryFromProto(&marketoverviewv1.GetSpotVolumeHistoryResponse{
		Bucket:     "15m",
		StartTsSec: 100,
		EndTsSec:   200,
		Points:     2,
		Pairs: []*marketoverviewv1.SpotPairVolumeSeries{{
			SymbolId:        7,
			VolumeUsdScaled: []int64{1, 2},
		}},
		TotalVolumeUsdScaled: []int64{3, 4},
	}, nil)
	if result.Bucket != "15m" || result.Points != 2 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Pairs) != 1 || result.Pairs[0].SymbolID != 7 {
		t.Fatalf("pairs=%+v", result.Pairs)
	}
	if got := result.Pairs[0].VolumeUsdScaled; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("pair volumes=%v", got)
	}
	if got := result.TotalVolumeUsdScaled; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("totals=%v", got)
	}
}

func TestEmptyOrderbookWithZeroSequenceIsSuccess(t *testing.T) {
	result, err := OrderbookFromProto(&orderbookv1.GetOrderBookResponse{SymbolId: 1, BookSeq: 0}, "BTC-USDT", 50, 8)
	if err != nil {
		t.Fatal(err)
	}
	if result.BookSeq != "0" {
		t.Fatalf("book_seq=%q", result.BookSeq)
	}
	if len(result.Bids) != 0 || len(result.Asks) != 0 {
		t.Fatalf("expected empty book, got %+v", result)
	}
}
