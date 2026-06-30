package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

func TestBalanceHistoryFromProto(t *testing.T) {
	msg := &ledgerrdv1.GetBalanceHistoryResponse{
		Range:      ledgerrdv1.BalanceRange_DAY_7,
		Bucket:     "1h",
		StartTsSec: 100,
		EndTsSec:   200,
		Points:     2,
		Series: []*ledgerrdv1.BalanceSeries{{
			AssetId:  1,
			BalanceQ: []uint64{100, 200},
		}},
	}
	result := decode.BalanceHistoryFromProto(msg)
	if result.Range != "7d" || result.Bucket != "1h" {
		t.Fatalf("range/bucket=%q/%q", result.Range, result.Bucket)
	}
	if len(result.Series) != 1 || len(result.Series[0].BalanceQ) != 2 {
		t.Fatalf("series=%+v", result.Series)
	}
}

func TestEquityHistoryFromProto(t *testing.T) {
	msg := &ledgerrdv1.GetEquityHistorySeriesResponse{
		Range:      ledgerrdv1.BalanceRange_DAY_30,
		Bucket:     "1d",
		StartTsSec: 1,
		EndTsSec:   2,
		QuoteAsset: "USD",
		Points:     1,
		Series: []*ledgerrdv1.EquitySeries{{
			Grouping: &ledgerrdv1.EquitySeries_Account{
				Account: &ledgerrdv1.AccountGrouping{AccountCode: 5, Name: "Trading"},
			},
			EquityQ: []int64{999},
		}},
	}
	result := decode.EquityHistoryFromProto(msg)
	if result.Range != "30d" || result.QuoteAsset != "USD" {
		t.Fatalf("result=%+v", result)
	}
	if result.Series[0].AccountCode != 5 || result.Series[0].AccountName != "Trading" {
		t.Fatalf("series=%+v", result.Series[0])
	}
}

func TestHoldsListFromProto(t *testing.T) {
	msg := &ledgerrdv1.ListHoldsResponse{
		Holds: []*ledgerrdv1.HoldRow{{
			HoldId:            42,
			AssetId:           1,
			AmountReservedE18: &typev1.U128{Lo: 500},
			ExpiresAtNs:       1_700_000_000_000,
		}},
	}
	result := decode.HoldsListFromProto(msg)
	if len(result.Holds) != 1 {
		t.Fatalf("holds=%+v", result.Holds)
	}
	if result.Holds[0].HoldID != codecs.FormatUint64ID(42) {
		t.Fatalf("hold_id=%q", result.Holds[0].HoldID)
	}
	if result.Holds[0].AmountReserved != "500" {
		t.Fatalf("amount_reserved=%q", result.Holds[0].AmountReserved)
	}
}
