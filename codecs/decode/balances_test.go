package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

func TestAssetBalanceFromProtoPreservesComponentRevisions(t *testing.T) {
	msg := &ledgerrdv1.AssetBalance{
		AssetId:         1,
		TradingRevision: 1<<63 + 1,
		FundingRevision: 1<<63 + 2,
	}
	result := decode.AssetBalanceFromProto(msg)
	if result.TradingRevision != msg.TradingRevision ||
		result.FundingRevision != msg.FundingRevision {
		t.Fatalf("revisions=%+v", result)
	}
}

func TestBalanceHistoryFromProto(t *testing.T) {
	// 1,844,674,407,370.955161549999999999: above the old 7-decimal uint64
	// ceiling, with all 18 decimals, so only an exact U128 decode keeps it.
	big := &typev1.U128{Hi: 99999999999, Lo: 0xfffffff45bc48bff}
	msg := &ledgerrdv1.GetBalanceHistoryResponse{
		Range:      ledgerrdv1.BalanceRange_DAY_7,
		Bucket:     "1h",
		StartTsSec: 100,
		EndTsSec:   200,
		Points:     3,
		Series: []*ledgerrdv1.BalanceSeries{{
			AssetId:     1,
			AccountCode: ledgerv1.AccountCode(-7),
			Balance:     []*typev1.U128{big, {Lo: 1}, {}},
		}},
	}
	result, err := decode.BalanceHistoryFromProto(msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Range != "7d" || result.Bucket != "1h" || result.Points != 3 {
		t.Fatalf("range/bucket/points=%q/%q/%d", result.Range, result.Bucket, result.Points)
	}
	if len(result.Series) != 1 || len(result.Series[0].Balance) != 3 {
		t.Fatalf("series=%+v", result.Series)
	}
	want := []string{"1844674407370955161549999999999", "1", "0"}
	wantText := []string{"1844674407370.955161549999999999", "0.000000000000000001", "0"}
	for i, b := range result.Series[0].Balance {
		if b.Scaled().String() != want[i] {
			t.Fatalf("balance[%d] scaled=%s, want %s", i, b.Scaled(), want[i])
		}
		if scale := b.Scale(); scale == nil || *scale != 18 {
			t.Fatalf("balance[%d] scale=%v", i, scale)
		}
		if id := b.AssetID(); id == nil || *id != 1 {
			t.Fatalf("balance[%d] asset=%v", i, id)
		}
		text, err := b.Format()
		if err != nil {
			t.Fatal(err)
		}
		if text != wantText[i] {
			t.Fatalf("balance[%d]=%s, want %s", i, text, wantText[i])
		}
	}
	if result.Series[0].AccountCode != -7 {
		t.Fatalf("unknown account code was not preserved: %d", result.Series[0].AccountCode)
	}
}

// TestBalanceHistoryRefusesMisalignedSeries pins the fail-closed rule: a
// series must hold one balance per point. A server on the previous contract
// sends balance_q, which this SDK no longer reads, so its series arrive empty.
func TestBalanceHistoryRefusesMisalignedSeries(t *testing.T) {
	for name, balances := range map[string][]*typev1.U128{
		"previous contract (none)": nil,
		"short":                    {{Lo: 1}},
		"long":                     {{Lo: 1}, {Lo: 2}, {Lo: 3}},
	} {
		msg := &ledgerrdv1.GetBalanceHistoryResponse{Points: 2, Series: []*ledgerrdv1.BalanceSeries{{AssetId: 4, Balance: balances}}}
		_, err := decode.BalanceHistoryFromProto(msg)
		var contractErr *sdkerrors.ResponseContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("%s: err=%v, want ResponseContractError", name, err)
		}
	}
	if _, err := decode.BalanceHistoryFromProto(&ledgerrdv1.GetBalanceHistoryResponse{}); err != nil {
		t.Fatalf("empty history: %v", err)
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
	if result.Series[0].PortfolioAccountID != "" || result.Series[0].PortfolioRemaining {
		t.Fatalf("account grouping leaked portfolio fields: %+v", result.Series[0])
	}
}

func TestEquityHistoryFromProtoPortfolioAccount(t *testing.T) {
	accountID := uint64(42)
	msg := &ledgerrdv1.GetEquityHistorySeriesResponse{
		Range:      ledgerrdv1.BalanceRange_DAY_7,
		QuoteAsset: "USDT",
		Points:     2,
		Series: []*ledgerrdv1.EquitySeries{
			{
				Grouping: &ledgerrdv1.EquitySeries_PortfolioAccount{
					PortfolioAccount: &ledgerrdv1.PortfolioAccountGrouping{AccountId: &accountID},
				},
				EquityQ: []int64{100, 200},
			},
			{
				Grouping: &ledgerrdv1.EquitySeries_PortfolioAccount{
					PortfolioAccount: &ledgerrdv1.PortfolioAccountGrouping{Remaining: true},
				},
				EquityQ: []int64{10, 20},
			},
		},
	}
	result := decode.EquityHistoryFromProto(msg)
	if len(result.Series) != 2 {
		t.Fatalf("series=%+v", result.Series)
	}
	if result.Series[0].PortfolioAccountID != codecs.FormatUint64ID(42) {
		t.Fatalf("named portfolio id=%q", result.Series[0].PortfolioAccountID)
	}
	if result.Series[0].PortfolioRemaining || result.Series[0].AccountCode != 0 {
		t.Fatalf("named series=%+v", result.Series[0])
	}
	if result.Series[1].PortfolioAccountID != "" || !result.Series[1].PortfolioRemaining {
		t.Fatalf("remaining series=%+v", result.Series[1])
	}
	if result.Series[0].EquityQ[0] != 100 || result.Series[1].EquityQ[1] != 20 {
		t.Fatalf("equity_q not preserved: %+v", result.Series)
	}
}
