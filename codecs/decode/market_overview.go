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

func decodedBaseVolume(value *int64, symbolID uint32, cats *catalogs.Manager) *string {
	if value == nil || cats == nil {
		return nil
	}
	scale, ok := cats.MarketDataVolumeScaleForSymbolID(symbolID)
	if !ok {
		return nil
	}
	formatted := formatQtyScaledOrEmpty(*value, scale)
	if formatted == "" {
		return nil
	}
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
		Volume24HBase:        decodedBaseVolume(m.Volume_24HBaseScaled, m.GetSymbolId(), cats),
		Volume24HQuoteScaled: optionalVolumeScaledString(m.Volume_24HQuoteScaled),
		Volume24HUsdScaled:   optionalVolumeScaledString(m.Volume_24HUsdScaled),
		Change24hBps:         m.GetChange_24HBps(),
	}
	for _, sparkline := range m.GetSparklines() {
		entry.Sparklines = append(entry.Sparklines, MarketOverviewSparklineFromProto(sparkline, symbol))
	}
	if ticks := m.GetLastPriceTicks(); ticks > 0 {
		entry.LastPrice = codecs.DecodePriceTicks(ticks, symbol)
	}
	if ticks := m.GetIndexPriceTicks(); ticks > 0 {
		entry.IndexPrice = codecs.DecodePriceTicks(ticks, symbol)
	}
	entry.High24h = optionalPrice(m.GetHigh_24HTicks(), symbol)
	entry.Low24h = optionalPrice(m.GetLow_24HTicks(), symbol)
	baseScale := -1
	if cats != nil {
		if scale, ok := cats.BaseQuantityScaleForSymbolID(m.GetSymbolId()); ok {
			baseScale = scale
		}
	}
	symbolID := m.GetSymbolId()
	entry.BestBid, entry.BestBidQty = bookSide(m.GetBestBidTicks(), m.GetBestBidQtyScaled(), baseScale, symbol, symbolID)
	entry.BestAsk, entry.BestAskQty = bookSide(m.GetBestAskTicks(), m.GetBestAskQtyScaled(), baseScale, symbol, symbolID)
	if ts := m.GetLastTradeTsNs(); ts != 0 {
		entry.LastTradeTsNs = strconv.FormatUint(ts, 10)
	}
	if ts := m.GetListedTsNs(); ts != 0 {
		entry.ListedTsNs = strconv.FormatUint(ts, 10)
	}
	return entry
}

// optionalPrice is a price the server sent, or nil for 0, which means none.
func optionalPrice(ticks int64, symbol string) *models.PriceTicks {
	if ticks <= 0 {
		return nil
	}
	price := codecs.DecodePriceTicks(ticks, symbol)
	return &price
}

// bookSide is the best price of one side of the book and the base quantity
// at it; both nil when that side is empty.
func bookSide(ticks, qtyScaled int64, scale int, symbol string, symbolID uint32) (*models.PriceTicks, *models.QtyScaled) {
	price := optionalPrice(ticks, symbol)
	if price == nil {
		return nil, nil
	}
	qty := codecs.DecodeQtyScaled(qtyScaled, scale, symbol, &symbolID)
	return price, &qty
}

// MarketOverviewSparklineFromProto decodes a sparkline's interval name and its
// close prices, newest first.
func MarketOverviewSparklineFromProto(m *marketoverviewv1.Sparkline, symbol string) models.MarketOverviewSparkline {
	closes := make([]models.PriceTicks, 0, len(m.GetCloseTicks()))
	for _, ticks := range m.GetCloseTicks() {
		closes = append(closes, codecs.DecodePriceTicks(ticks, symbol))
	}
	return models.MarketOverviewSparkline{Interval: m.GetInterval().String(), ClosePrices: closes}
}

func MarketOverviewListFromProto(msg *marketoverviewv1.ListMarketOverviewResponse, cats *catalogs.Manager) models.MarketOverviewList {
	out := make([]models.MarketOverviewEntry, 0, len(msg.GetMarkets()))
	for _, m := range msg.GetMarkets() {
		out = append(out, MarketOverviewEntryFromProto(m, cats))
	}
	return models.MarketOverviewList{Markets: out, NextPageToken: msg.GetNextPageToken()}
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
