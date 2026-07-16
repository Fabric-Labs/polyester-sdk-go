package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var balanceRangeLabels = map[ledgerrdv1.BalanceRange]string{
	ledgerrdv1.BalanceRange_DAY_1: "1d", ledgerrdv1.BalanceRange_DAY_7: "7d",
	ledgerrdv1.BalanceRange_DAY_30: "30d", ledgerrdv1.BalanceRange_DAY_90: "90d",
	ledgerrdv1.BalanceRange_DAY_180: "180d", ledgerrdv1.BalanceRange_DAY_365: "365d",
}

func u128(msg *typev1.U128) string {
	if msg == nil {
		return "0"
	}
	return codecs.U128ToStr(msg.GetHi(), msg.GetLo())
}

func AssetBalanceFromProto(msg *ledgerrdv1.AssetBalance) models.AssetBalance {
	return models.AssetBalance{
		AssetID:         msg.GetAssetId(),
		Trading:         u128(msg.GetTrading()),
		Funding:         u128(msg.GetFunding()),
		Reserved:        u128(msg.GetReserved()),
		Available:       u128(msg.GetAvailable()),
		TradingVersion:  msg.GetTradingVersion(),
		FundingVersion:  msg.GetFundingVersion(),
		ReservedVersion: msg.GetReservedVersion(),
	}
}

func BalancesListFromProto(msg *ledgerrdv1.GetBalancesResponse) models.BalancesList {
	out := make([]models.AssetBalance, 0, len(msg.GetBalances()))
	for _, b := range msg.GetBalances() {
		out = append(out, AssetBalanceFromProto(b))
	}
	return models.BalancesList{Balances: out}
}

func BalanceHistoryFromProto(msg *ledgerrdv1.GetBalanceHistoryResponse) models.BalanceHistory {
	series := make([]models.BalanceHistorySeries, 0, len(msg.GetSeries()))
	for _, s := range msg.GetSeries() {
		series = append(series, models.BalanceHistorySeries{AssetID: s.GetAssetId(), AccountCode: uint32(s.GetAccountCode()), BalanceQ: uintsToInts(s.GetBalanceQ())})
	}
	return models.BalanceHistory{Range: balanceRangeLabels[msg.GetRange()], Bucket: msg.GetBucket(), StartTsSec: int64(msg.GetStartTsSec()), EndTsSec: int64(msg.GetEndTsSec()), Points: int(msg.GetPoints()), Series: series}
}

func uintsToInts(in []uint64) []int64 {
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}

func EquityHistoryFromProto(msg *ledgerrdv1.GetEquityHistorySeriesResponse) models.EquityHistory {
	series := make([]models.EquityHistorySeries, 0, len(msg.GetSeries()))
	for _, s := range msg.GetSeries() {
		row := models.EquityHistorySeries{EquityQ: append([]int64(nil), s.GetEquityQ()...)}
		if a := s.GetAccount(); a != nil {
			row.AccountCode = uint32(a.GetAccountCode())
			row.AccountName = a.GetName()
		}
		if a := s.GetAsset(); a != nil {
			row.AssetID = a.GetId()
			row.AssetSymbol = a.GetSymbol()
		}
		series = append(series, row)
	}
	return models.EquityHistory{Range: balanceRangeLabels[msg.GetRange()], Bucket: msg.GetBucket(), StartTsSec: int64(msg.GetStartTsSec()), EndTsSec: int64(msg.GetEndTsSec()), QuoteAsset: msg.GetQuoteAsset(), Points: int(msg.GetPoints()), Series: series}
}

func HoldFromProto(msg *ledgerrdv1.HoldRow) models.Hold {
	return models.Hold{HoldID: codecs.FormatUint64ID(msg.GetHoldId()), AssetID: msg.GetAssetId(), AmountReserved: u128(msg.GetAmountReservedE18()), ExpiresAtNs: strconv.FormatUint(msg.GetExpiresAtNs(), 10)}
}

func HoldsListFromProto(msg *ledgerrdv1.ListHoldsResponse) models.HoldsList {
	out := make([]models.Hold, 0, len(msg.GetHolds()))
	for _, h := range msg.GetHolds() {
		out = append(out, HoldFromProto(h))
	}
	return models.HoldsList{Holds: out}
}
