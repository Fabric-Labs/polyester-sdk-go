package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func optionalVolumeScaledString(value *int64) *string {
	if value == nil {
		return nil
	}
	formatted := strconv.FormatInt(*value, 10)
	return &formatted
}

func MarketOverviewEntryFromProto(m *marketoverviewv1.MarketOverview, cats *catalogs.Manager) models.MarketOverviewEntry {
	if m == nil {
		return models.MarketOverviewEntry{}
	}
	symbol := catalogSymbol(cats, m.GetSymbolId())
	entry := models.MarketOverviewEntry{
		SymbolID:             m.GetSymbolId(),
		Symbol:               symbol,
		Volume24HBaseScaled:  optionalVolumeScaledString(m.Volume_24HBaseScaled),
		Volume24HQuoteScaled: optionalVolumeScaledString(m.Volume_24HQuoteScaled),
		Volume24HUsdScaled:   optionalVolumeScaledString(m.Volume_24HUsdScaled),
	}
	if ticks := m.GetLastPriceTicks(); ticks > 0 {
		entry.LastPrice = codecs.DecodePriceTicks(ticks, symbol)
	}
	if ticks := m.GetIndexPriceTicks(); ticks > 0 {
		entry.IndexPrice = codecs.DecodePriceTicks(ticks, symbol)
	}
	return entry
}

func MarketOverviewListFromProto(msg *marketoverviewv1.ListMarketOverviewResponse, cats *catalogs.Manager) models.MarketOverviewList {
	out := make([]models.MarketOverviewEntry, 0, len(msg.GetMarkets()))
	for _, m := range msg.GetMarkets() {
		out = append(out, MarketOverviewEntryFromProto(m, cats))
	}
	return models.MarketOverviewList{Markets: out}
}

func SpotVolumeHistoryFromProto(msg *marketoverviewv1.GetSpotVolumeHistoryResponse, cats *catalogs.Manager) models.SpotVolumeHistory {
	if msg == nil {
		return models.SpotVolumeHistory{}
	}
	pairs := make([]models.SpotPairVolumeSeries, 0, len(msg.GetPairs()))
	for _, item := range msg.GetPairs() {
		if item == nil {
			continue
		}
		pairs = append(pairs, models.SpotPairVolumeSeries{
			SymbolID:        item.GetSymbolId(),
			Symbol:          catalogSymbol(cats, item.GetSymbolId()),
			VolumeUsdScaled: append([]int64(nil), item.GetVolumeUsdScaled()...),
		})
	}
	return models.SpotVolumeHistory{
		Bucket:               msg.GetBucket(),
		StartTsSec:           msg.GetStartTsSec(),
		EndTsSec:             msg.GetEndTsSec(),
		Points:               msg.GetPoints(),
		Pairs:                pairs,
		TotalVolumeUsdScaled: append([]int64(nil), msg.GetTotalVolumeUsdScaled()...),
	}
}

func CurrencyMetadataFromProto(m *marketoverviewv1.CurrencyMetadata) models.CurrencyMetadata {
	if m == nil {
		return models.CurrencyMetadata{}
	}
	return models.CurrencyMetadata{
		Code:               m.GetCode(),
		DefaultEnglishName: m.GetDefaultEnglishName(),
		Symbol:             m.GetSymbol(),
		FractionDigits:     m.GetFractionDigits(),
	}
}

func CurrencyConversionConfigFromProto(msg *marketoverviewv1.GetCurrencyConversionConfigResponse) models.CurrencyConversionConfig {
	if msg == nil {
		return models.CurrencyConversionConfig{}
	}
	fiat := make([]models.CurrencyMetadata, 0, len(msg.GetFiat()))
	for _, item := range msg.GetFiat() {
		fiat = append(fiat, CurrencyMetadataFromProto(item))
	}
	stablecoins := make([]models.CurrencyMetadata, 0, len(msg.GetStablecoins()))
	for _, item := range msg.GetStablecoins() {
		stablecoins = append(stablecoins, CurrencyMetadataFromProto(item))
	}
	return models.CurrencyConversionConfig{Fiat: fiat, Stablecoins: stablecoins}
}

func FiatConversionSnapshotFromProto(msg *marketoverviewv1.FiatConversionSnapshot) *models.FiatConversionSnapshot {
	if msg == nil {
		return nil
	}
	rates := make([]models.FiatConversionRate, 0, len(msg.GetRates()))
	for _, item := range msg.GetRates() {
		if item == nil {
			continue
		}
		rates = append(rates, models.FiatConversionRate{
			Code:          item.GetCode(),
			UnitsPerUsdE8: item.GetUnitsPerUsdE8(),
		})
	}
	return &models.FiatConversionSnapshot{
		Rates:       rates,
		SourceTsSec: msg.GetSourceTsSec(),
		Stale:       msg.GetStale(),
	}
}

func CurrencyConversionRatesFromProto(msg *marketoverviewv1.GetCurrencyConversionRatesResponse) models.CurrencyConversionRates {
	if msg == nil {
		return models.CurrencyConversionRates{}
	}
	stablecoins := make([]models.StablecoinConversionRate, 0, len(msg.GetStablecoins()))
	for _, item := range msg.GetStablecoins() {
		if item == nil {
			continue
		}
		stablecoins = append(stablecoins, models.StablecoinConversionRate{
			Code:         item.GetCode(),
			UsdPerUnitE8: item.GetUsdPerUnitE8(),
			SourceTsSec:  item.GetSourceTsSec(),
			Stale:        item.GetStale(),
		})
	}
	return models.CurrencyConversionRates{
		Fiat:          FiatConversionSnapshotFromProto(msg.GetFiat()),
		Stablecoins:   stablecoins,
		SnapshotTsSec: msg.GetSnapshotTsSec(),
	}
}
