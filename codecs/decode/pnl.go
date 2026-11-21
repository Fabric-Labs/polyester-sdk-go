package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	pnlv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func profitAndLossNotices(values []pnlv1.ProfitAndLossNotice) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, codecs.ProtoEnumName(value, value.Number()))
	}
	return out
}

func profitAndLossProduct(value pnlv1.ProfitAndLossProduct) string {
	return codecs.ProtoEnumName(value, value.Number())
}

// ProfitAndLossFromProto decodes GetProfitAndLossResponse. Optional figures
// stay nil when the server omits them.
func ProfitAndLossFromProto(msg *pnlv1.GetProfitAndLossResponse) models.ProfitAndLoss {
	out := models.ProfitAndLoss{
		WindowStart:            timestampTime(msg.GetWindowStart()),
		AsOf:                   timestampTime(msg.GetAsOf()),
		MarkTime:               timestampTime(msg.GetMarkTime()),
		PnLUSD:                 cloneOptionalString(msg.PnlUsd),
		RealizedPnLUSD:         msg.GetRealizedPnlUsd(),
		UnrealizedPnLChangeUSD: cloneOptionalString(msg.UnrealizedPnlChangeUsd),
		UnrealizedPnLUSD:       cloneOptionalString(msg.UnrealizedPnlUsd),
		FeesUSD:                msg.GetFeesUsd(),
		VolumeUSD:              msg.GetVolumeUsd(),
		Products:               make([]models.ProductProfitAndLoss, 0, len(msg.GetProducts())),
		Markets:                make([]models.MarketProfitAndLoss, 0, len(msg.GetMarkets())),
		Holdings:               make([]models.HoldingProfitAndLoss, 0, len(msg.GetHoldings())),
		Notices:                profitAndLossNotices(msg.GetNotices()),
	}
	for _, p := range msg.GetProducts() {
		out.Products = append(out.Products, models.ProductProfitAndLoss{
			Product:        profitAndLossProduct(p.GetProduct()),
			RealizedPnLUSD: p.GetRealizedPnlUsd(),
			FeesUSD:        p.GetFeesUsd(),
			VolumeUSD:      p.GetVolumeUsd(),
		})
	}
	for _, m := range msg.GetMarkets() {
		out.Markets = append(out.Markets, models.MarketProfitAndLoss{
			SymbolID:       m.GetSymbolId(),
			Product:        profitAndLossProduct(m.GetProduct()),
			RealizedPnLUSD: m.GetRealizedPnlUsd(),
			FeesUSD:        m.GetFeesUsd(),
			VolumeUSD:      m.GetVolumeUsd(),
			TradeCount:     m.GetTradeCount(),
		})
	}
	for _, h := range msg.GetHoldings() {
		out.Holdings = append(out.Holdings, models.HoldingProfitAndLoss{
			AssetID:           h.GetAssetId(),
			PositionQuantity:  h.GetPositionQuantity(),
			UntrackedQuantity: h.GetUntrackedQuantity(),
			CostUSD:           h.GetCostUsd(),
			EntryPriceUSD:     cloneOptionalString(h.EntryPriceUsd),
			MarkPriceUSD:      cloneOptionalString(h.MarkPriceUsd),
			UnrealizedPnLUSD:  cloneOptionalString(h.UnrealizedPnlUsd),
		})
	}
	return out
}

// DailyProfitAndLossListFromProto decodes ListDailyProfitAndLossResponse.
func DailyProfitAndLossListFromProto(msg *pnlv1.ListDailyProfitAndLossResponse) models.DailyProfitAndLossList {
	days := make([]models.DailyProfitAndLoss, 0, len(msg.GetDays()))
	for _, d := range msg.GetDays() {
		days = append(days, models.DailyProfitAndLoss{
			Day:                     timestampTime(d.GetDay()),
			PnLUSD:                  cloneOptionalString(d.PnlUsd),
			RealizedPnLUSD:          d.GetRealizedPnlUsd(),
			UnrealizedPnLAtCloseUSD: cloneOptionalString(d.UnrealizedPnlAtCloseUsd),
			FeesUSD:                 d.GetFeesUsd(),
			VolumeUSD:               d.GetVolumeUsd(),
			TradeCount:              d.GetTradeCount(),
			Notices:                 profitAndLossNotices(d.GetNotices()),
			StartEquityUSD:          cloneOptionalString(d.StartEquityUsd),
			EndEquityUSD:            cloneOptionalString(d.EndEquityUsd),
			NetFlowUSD:              cloneOptionalString(d.NetFlowUsd),
		})
	}
	return models.DailyProfitAndLossList{Days: days, NextPageToken: msg.GetNextPageToken(), AsOf: timestampTime(msg.GetAsOf())}
}

// TradingStatisticsFromProto decodes GetTradingStatisticsResponse.
func TradingStatisticsFromProto(msg *pnlv1.GetTradingStatisticsResponse) models.TradingStatistics {
	return models.TradingStatistics{
		WindowStart:    timestampTime(msg.GetWindowStart()),
		AsOf:           timestampTime(msg.GetAsOf()),
		TradeCount:     msg.GetTradeCount(),
		VolumeUSD:      msg.GetVolumeUsd(),
		FeesUSD:        msg.GetFeesUsd(),
		RealizedPnLUSD: msg.GetRealizedPnlUsd(),
		Notices:        profitAndLossNotices(msg.GetNotices()),
		ReturnPct:      cloneOptionalString(msg.ReturnPct),
		MaxDrawdownPct: cloneOptionalString(msg.MaxDrawdownPct),
		Sharpe:         cloneOptionalString(msg.Sharpe),
		ReturnDays:     msg.GetReturnDays(),
	}
}
