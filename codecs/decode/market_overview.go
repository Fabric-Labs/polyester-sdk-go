package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func MarketOverviewEntryFromProto(m *marketoverviewv1.MarketOverview, cats *catalogs.Manager) models.MarketOverviewEntry {
	if m == nil {
		return models.MarketOverviewEntry{}
	}
	symbol := catalogSymbol(cats, m.GetSymbolId())
	entry := models.MarketOverviewEntry{SymbolID: m.GetSymbolId(), Symbol: symbol}
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
