package services

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

const (
	snapshotTradeTs = "1795721087978971514"
	olderTradeTs    = "1795721087185742363"
	newerTradeTs    = "1795721088000000000"
)

// overviewTestCallbacks are the callbacks CreateSubscription gives the
// snapshot coordinator, over a fresh merge, and the last view they emitted.
func overviewTestCallbacks() (overviewCallbacks, func() map[uint32]models.MarketOverviewEntry) {
	merged := newOverviewMerge(func(models.MarketOverviewEntry) bool { return true })
	var last []models.MarketOverviewEntry
	callbacks := overviewStreamCallbacks(merged, func() { last = merged.rows() })
	return callbacks, func() map[uint32]models.MarketOverviewEntry {
		view := map[uint32]models.MarketOverviewEntry{}
		for _, row := range last {
			view[row.SymbolID] = row
		}
		return view
	}
}

func overviewEntry(symbolID uint32, tradeTs string, change int32) models.MarketOverviewEntry {
	return models.MarketOverviewEntry{SymbolID: symbolID, LastTradeTsNs: tradeTs, Change24hBps: change}
}

func overviewBatch(rows ...models.MarketOverviewEntry) []models.MarketOverviewList {
	return []models.MarketOverviewList{{Markets: rows}}
}

func snapshotOf(rows ...models.MarketOverviewEntry) models.MarketOverviewList {
	return models.MarketOverviewList{Markets: rows}
}

// Rows buffered while the snapshot loaded are merged by freshness: an older
// one does not replace the snapshot's newer row; a newer one does, and so does
// one with the same last-trade time, with its changes; a market the snapshot
// did not have is added.
func TestOverviewBufferedRowsAreMergedByFreshness(t *testing.T) {
	volume := "250"
	equal := overviewEntry(1, snapshotTradeTs, 7)
	equal.Volume24HUsdScaled = &volume
	for name, tc := range map[string]struct {
		buffered    models.MarketOverviewEntry
		wantTradeTs string
		wantChange  int32
	}{
		"older":           {overviewEntry(1, olderTradeTs, 3), snapshotTradeTs, 0},
		"newer":           {overviewEntry(1, newerTradeTs, 5), newerTradeTs, 5},
		"equal timestamp": {equal, snapshotTradeTs, 7},
	} {
		callbacks, view := overviewTestCallbacks()
		callbacks.applySnapshot(snapshotOf(overviewEntry(1, snapshotTradeTs, 0)), overviewBatch(tc.buffered, overviewEntry(2, olderTradeTs, 1)))
		got := view()
		if got[1].LastTradeTsNs != tc.wantTradeTs || got[1].Change24hBps != tc.wantChange {
			t.Fatalf("%s: row = %+v, want trade %s change %d", name, got[1], tc.wantTradeTs, tc.wantChange)
		}
		if name == "equal timestamp" && (got[1].Volume24HUsdScaled == nil || *got[1].Volume24HUsdScaled != "250") {
			t.Fatalf("equal timestamp: volume = %v, want the buffered row's", got[1].Volume24HUsdScaled)
		}
		if _, ok := got[2]; !ok {
			t.Fatalf("%s: a buffered market missing from the snapshot was not added", name)
		}
	}
}

// Publications drained while the snapshot is being applied are merged with the
// same guard; publications the coordinator dispatched live apply as they come,
// an older last-trade time included.
func TestOverviewDrainedPublicationsAreGuardedAndLiveOnesAreNot(t *testing.T) {
	callbacks, view := overviewTestCallbacks()
	callbacks.applySnapshot(snapshotOf(overviewEntry(1, snapshotTradeTs, 0)), nil)

	callbacks.applyDrained(overviewBatch(overviewEntry(1, olderTradeTs, 3)))
	if got := view()[1]; got.LastTradeTsNs != snapshotTradeTs {
		t.Fatalf("drained: an older row replaced the snapshot row: %+v", got)
	}
	callbacks.applyDrained(overviewBatch(overviewEntry(1, snapshotTradeTs, 7)))
	if got := view()[1]; got.Change24hBps != 7 {
		t.Fatalf("drained: an equal-trade-time row did not apply: %+v", got)
	}
	callbacks.applyLive(overviewBatch(overviewEntry(1, olderTradeTs, 9)))
	if got := view()[1]; got.LastTradeTsNs != olderTradeTs || got.Change24hBps != 9 {
		t.Fatalf("live: a live row did not apply as before: %+v", got)
	}
}
