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
	"google.golang.org/protobuf/proto"
)

func TestAssetBalanceFromProtoPreservesFundingRevision(t *testing.T) {
	msg := &ledgerrdv1.AssetBalance{AssetId: 1, FundingRevision: 1<<63 + 2}
	result := decode.AssetBalanceFromProto(msg)
	if result.FundingRevision != msg.FundingRevision {
		t.Fatalf("revision=%+v", result)
	}
}

// GetBalances returns the position its Trading values are at; the full
// unsigned 64-bit range survives the decode.
func TestBalancesListFromProtoKeepsTradingPosition(t *testing.T) {
	msg := &ledgerrdv1.GetBalancesResponse{
		Balances:                  []*ledgerrdv1.AssetBalance{{AssetId: 1, Trading: &typev1.U128{Lo: 5}, FundingRevision: 81}},
		TradingEpoch:              1<<63 + 7,
		TradingSequence:           42,
		TradingAppliedThroughTsNs: 1767225600123456789,
	}
	result := decode.BalancesListFromProto(msg)
	if result.TradingEpoch != 1<<63+7 || result.TradingSequence != 42 || result.TradingAppliedThroughTsNs != 1767225600123456789 {
		t.Fatalf("position=%+v", result)
	}
	if len(result.Balances) != 1 || result.Balances[0].Trading != "5" || result.Balances[0].FundingRevision != 81 {
		t.Fatalf("balances=%+v", result.Balances)
	}
	if empty := decode.BalancesListFromProto(&ledgerrdv1.GetBalancesResponse{}); empty.TradingEpoch != 0 || empty.TradingSequence != 0 || empty.Balances == nil {
		t.Fatalf("empty=%+v", empty)
	}
}

// The balance stream delivers the generated BalanceUpdate unchanged: TRADING
// updates keep their position, completeness, assets and legs; FUNDING
// updates keep their revision.
func TestBalanceUpdateFromBytesKeepsPublicationAsSent(t *testing.T) {
	trading := &ledgerrdv1.BalanceUpdate{
		Account:                   ledgerv1.AccountCode_TRADING,
		TradingEpoch:              4294967303,
		TradingSequence:           43,
		Complete:                  true,
		TradingAppliedThroughTsNs: 1767225600123456789,
		Balances:                  []*ledgerrdv1.AssetBalance{{AssetId: 1, Trading: &typev1.U128{Lo: 250}, Reserved: &typev1.U128{Lo: 25}, Available: &typev1.U128{Lo: 225}}},
		Transfers:                 []*ledgerrdv1.TransferRow{{TransferId: "340282366920938463463374607431768211455", AssetId: 1, IsDebit: true, TsNs: 1767225600123456789}},
	}
	funding := &ledgerrdv1.BalanceUpdate{
		Account:  ledgerv1.AccountCode_FUNDING,
		Balances: []*ledgerrdv1.AssetBalance{{AssetId: 1, Funding: &typev1.U128{Lo: 1000}, FundingRevision: 81}},
	}
	for _, want := range []*ledgerrdv1.BalanceUpdate{trading, funding} {
		payload, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decode.BalanceUpdateFromBytes(payload)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if _, err := decode.BalanceUpdateFromBytes(nil); err == nil {
		t.Fatal("empty publication decoded")
	}
	if _, err := decode.BalanceUpdateFromBytes([]byte{0xff}); err == nil {
		t.Fatal("malformed publication decoded")
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
