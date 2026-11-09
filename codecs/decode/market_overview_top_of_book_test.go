package decode

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// The Devnet BTC-USDT row from POLY-9019.
func topOfBookRow() *overviewpb.MarketOverview {
	return &overviewpb.MarketOverview{
		SymbolId:         1,
		LastPriceTicks:   74900000000000,
		High_24HTicks:    76167500000000,
		Low_24HTicks:     73621960000000,
		BestBidTicks:     74883900000000,
		BestBidQtyScaled: 6166686,
		BestAskTicks:     74921360000000,
		BestAskQtyScaled: 5532396,
		LastTradeTsNs:    1794236100652374117,
		ListedTsNs:       1792404511505163000,
	}
}

func topOfBookCatalogs(t *testing.T) *catalogs.Manager {
	t.Helper()
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{
		"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbolId": float64(1), "baseQuantityScale": float64(8)}},
	}); err != nil {
		t.Fatal(err)
	}
	return cats
}

func wantPrice(t *testing.T, name string, got *models.PriceTicks, ticks int64, symbol string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is unset, want %d ticks", name, ticks)
	}
	if got.Ticks() != ticks || got.Symbol() != symbol {
		t.Fatalf("%s = %d %q, want %d %q", name, got.Ticks(), got.Symbol(), ticks, symbol)
	}
}

func wantQty(t *testing.T, name string, got *models.QtyScaled, scaled int64, scale int, symbol string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is unset, want %d", name, scaled)
	}
	if got.Scaled() != scaled || got.Scale() == nil || *got.Scale() != scale || got.Symbol() != symbol ||
		got.SymbolID() == nil || *got.SymbolID() != 1 || got.Domain() != models.QuantityDomainOrderBase {
		t.Fatalf("%s = %d scale %v %q, want %d scale %d %q", name, got.Scaled(), got.Scale(), got.Symbol(), scaled, scale, symbol)
	}
}

func assertTopOfBook(t *testing.T, row models.MarketOverviewEntry) {
	t.Helper()
	wantPrice(t, "High24h", row.High24h, 76167500000000, "BTC-USDT")
	wantPrice(t, "Low24h", row.Low24h, 73621960000000, "BTC-USDT")
	wantPrice(t, "BestBid", row.BestBid, 74883900000000, "BTC-USDT")
	wantQty(t, "BestBidQty", row.BestBidQty, 6166686, 8, "BTC-USDT")
	wantPrice(t, "BestAsk", row.BestAsk, 74921360000000, "BTC-USDT")
	wantQty(t, "BestAskQty", row.BestAskQty, 5532396, 8, "BTC-USDT")
	if row.LastTradeTsNs != "1794236100652374117" || row.ListedTsNs != "1792404511505163000" {
		t.Fatalf("times = %q %q", row.LastTradeTsNs, row.ListedTsNs)
	}
	if row.LastPrice.Ticks() != 74900000000000 {
		t.Fatalf("last price changed: %d", row.LastPrice.Ticks())
	}
}

// POLY-9019: list rows keep the 24h range, top of book and times.
func TestMarketOverviewListKeepsRangeTopOfBookAndTimes(t *testing.T) {
	list := MarketOverviewListFromProto(&overviewpb.ListMarketOverviewResponse{
		Markets: []*overviewpb.MarketOverview{topOfBookRow()},
	}, topOfBookCatalogs(t))
	assertTopOfBook(t, list.Markets[0])
}

// POLY-9019: live publications keep them too.
func TestMarketOverviewPublicationsKeepRangeTopOfBookAndTimes(t *testing.T) {
	payload, err := proto.Marshal(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{topOfBookRow()}})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := MarketOverviewBatchDecoder(topOfBookCatalogs(t))(payload)
	if err != nil {
		t.Fatal(err)
	}
	assertTopOfBook(t, batch.Markets[0])
}

// POLY-9019: an empty book side, and a market with no trade or listing time,
// decode as unset rather than zero.
func TestMarketOverviewEmptySideAndTimesAreUnset(t *testing.T) {
	for _, side := range []string{"bid", "ask"} {
		t.Run(side, func(t *testing.T) {
			msg := topOfBookRow()
			msg.High_24HTicks, msg.Low_24HTicks, msg.LastTradeTsNs, msg.ListedTsNs = 0, 0, 0, 0
			if side == "bid" {
				msg.BestBidTicks, msg.BestBidQtyScaled = 0, 0
			} else {
				msg.BestAskTicks, msg.BestAskQtyScaled = 0, 0
			}
			row := MarketOverviewEntryFromProto(msg, topOfBookCatalogs(t))
			if row.High24h != nil || row.Low24h != nil || row.LastTradeTsNs != "" || row.ListedTsNs != "" {
				t.Fatalf("unset range or times decoded as set: %+v", row)
			}
			empty := row.BestBid == nil && row.BestBidQty == nil
			other := row.BestAsk != nil && row.BestAskQty != nil
			if side == "ask" {
				empty = row.BestAsk == nil && row.BestAskQty == nil
				other = row.BestBid != nil && row.BestBidQty != nil
			}
			if !empty || !other {
				t.Fatalf("empty %s side decoded wrongly: %+v", side, row)
			}
		})
	}
}
