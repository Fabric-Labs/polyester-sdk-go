package decode

import (
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func MarketOverviewEntryFromProto(m *marketoverviewv1.MarketOverview) models.MarketOverviewEntry {
	if m == nil {
		return models.MarketOverviewEntry{}
	}
	return models.MarketOverviewEntry{SymbolID: m.GetSymbolId(), Symbol: m.GetSymbol()}
}

func MarketOverviewListFromProto(msg *marketoverviewv1.ListMarketOverviewResponse) models.MarketOverviewList {
	out := make([]models.MarketOverviewEntry, 0, len(msg.GetMarkets()))
	for _, m := range msg.GetMarkets() {
		out = append(out, MarketOverviewEntryFromProto(m))
	}
	return models.MarketOverviewList{Markets: out}
}
