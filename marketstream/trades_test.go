package marketstream

import (
	"reflect"
	"testing"
)

const e2 = uint64(1) << 40 // base of epoch 2
const e3 = uint64(2) << 40 // base of epoch 3

func thr(epoch, last uint64) *tradesThroughMsg {
	return &tradesThroughMsg{symbolID: 7, epoch: epoch, last: last}
}

func skip(epoch, first, last uint64) *tradesSkippedMsg {
	return &tradesSkippedMsg{symbolID: 7, epoch: epoch, first: first, last: last}
}

func trd(epoch uint64, ids ...uint64) *tradesMsg {
	m := &tradesMsg{symbolID: 7, epoch: epoch}
	for _, id := range ids {
		m.fills = append(m.fills, fill(id))
	}
	return m
}

func g(after, through, tail uint64, cause GapCause) *TradesGap {
	return &TradesGap{SymbolID: 7, AfterTradeID: after, ThroughTradeID: through, TailEpoch: tail, Cause: cause}
}

func ids(fills []fillMsg) []uint64 {
	out := []uint64{}
	for _, f := range fills {
		out = append(out, f.tradeID)
	}
	return out
}

// step applies one message and checks the delivered trade IDs and gaps.
func step(t *testing.T, tr *tradeTracker, msg any, wantIDs []uint64, wantGaps ...*TradesGap) {
	t.Helper()
	var (
		fills []fillMsg
		gaps  []*TradesGap
		err   error
	)
	switch m := msg.(type) {
	case *tradesThroughMsg:
		gaps, err = tr.through(m)
	case *tradesSkippedMsg:
		gaps, err = tr.skipped(m)
	case *tradesMsg:
		fills, gaps, err = tr.trades(m)
	}
	if err != nil {
		t.Fatalf("%+v: %v", msg, err)
	}
	if got := ids(fills); !reflect.DeepEqual(got, append([]uint64{}, wantIDs...)) {
		t.Fatalf("%+v delivered %v, want %v", msg, got, wantIDs)
	}
	if len(gaps) != len(wantGaps) {
		t.Fatalf("%+v gaps %+v, want %+v", msg, deref(gaps), deref(wantGaps))
	}
	for i := range gaps {
		if *gaps[i] != *wantGaps[i] {
			t.Fatalf("%+v gap %d is %+v, want %+v", msg, i, *gaps[i], *wantGaps[i])
		}
	}
}

func deref(gaps []*TradesGap) []TradesGap {
	out := make([]TradesGap, len(gaps))
	for i, g := range gaps {
		out[i] = *g
	}
	return out
}

func TestTradesContiguousAfterBaseline(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	step(t, tr, trd(1, 101, 102), []uint64{101, 102})
	step(t, tr, trd(1, 103), []uint64{103})
	step(t, tr, thr(1, 103), nil)
	step(t, tr, thr(1, 102), nil) // an older frontier only confirms
}

func TestTradesBeforeBaselineAreAProtocolError(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	if _, _, err := tr.trades(trd(1, 1)); err == nil {
		t.Fatal("trades before the baseline accepted")
	}
	if _, err := tr.skipped(skip(1, 1, 2)); err == nil {
		t.Fatal("skipped before the baseline accepted")
	}
}

// The server's rejoin schedule: L = 100, trades 101..105 lost.
func TestTradesServerSkippedRanges(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	step(t, tr, skip(1, 101, 107), nil, g(100, 107, 0, GapSkipped))
	step(t, tr, trd(1, 108, 109, 110), []uint64{108, 109, 110})
	// A duplicate range is ignored; a duplicate trade is not delivered again.
	step(t, tr, skip(1, 101, 105), nil)
	step(t, tr, trd(1, 109, 110, 111), []uint64{111})
}

func TestTradesGapsTheClientDetects(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	// A trade past the next ID.
	step(t, tr, trd(1, 103, 104), []uint64{103, 104}, g(100, 102, 0, GapDetected))
	// A jump inside one message.
	step(t, tr, trd(1, 105, 107), []uint64{105, 107}, g(105, 106, 0, GapDetected))
	// A frontier past the last trade accounted for.
	step(t, tr, thr(1, 110), nil, g(107, 110, 0, GapDetected))
	// A skipped range that does not start at the next ID.
	step(t, tr, skip(1, 113, 114), nil, g(110, 112, 0, GapDetected), g(112, 114, 0, GapSkipped))
}

func TestTradesEpochChangeWithReportedTail(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	step(t, tr, skip(1, 101, 0), nil, g(100, 0, 1, GapSkipped))
	// Late trades of the closed epoch are covered by the tail's recovery.
	step(t, tr, trd(1, 101), nil)
	step(t, tr, trd(2, e2+1, e2+2), []uint64{e2 + 1, e2 + 2})
	step(t, tr, thr(2, e2+2), nil)
}

func TestTradesEpochChangeWithoutReportedTail(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	// Epoch 2's first trade with no tail for epoch 1: everything after 100
	// through epoch 2's base is unaccounted.
	step(t, tr, trd(2, e2+1), []uint64{e2 + 1}, g(100, e2, 0, GapDetected))
	// Epoch 2's tail reported, then epoch 4 skipping epoch 3 entirely.
	step(t, tr, skip(2, e2+2, 0), nil, g(e2+1, 0, 2, GapSkipped))
	e4 := uint64(3) << 40
	step(t, tr, thr(4, e4+5), nil, g(e3, e4, 0, GapDetected), g(e4, e4+5, 0, GapDetected))
}

func TestTradesStaleEpochIsIgnored(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(2, e2+10), nil)
	step(t, tr, trd(1, 50), nil)
	step(t, tr, skip(1, 40, 60), nil)
	step(t, tr, thr(1, 60), nil)
}

func TestTradesReconnectReconcilesTheNewBaseline(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	step(t, tr, thr(1, 100), nil)
	step(t, tr, trd(1, 101), []uint64{101})

	tr.restart()
	step(t, tr, thr(2, e2+3), nil, g(101, e2+3, 0, GapReconnect))
	step(t, tr, trd(2, e2+4), []uint64{e2 + 4})

	// A replica behind the last connection: its trades up to what was
	// delivered are dropped, never delivered twice.
	tr.restart()
	step(t, tr, thr(2, e2+2), nil)
	step(t, tr, trd(2, e2+3, e2+4, e2+5), []uint64{e2 + 5})
}

func TestTradesOutOfEpochIDsAreAProtocolError(t *testing.T) {
	tr := &tradeTracker{symbolID: 7}
	if _, err := tr.through(thr(2, 5)); err == nil {
		t.Fatal("frontier of epoch 1 under epoch 2 accepted")
	}
	step(t, tr, thr(1, 100), nil)
	for _, m := range []*tradesMsg{trd(1, e2+1), trd(1, 102, 101), trd(0, 5), {symbolID: 7, epoch: 1}} {
		if _, _, err := tr.trades(m); err == nil {
			t.Fatalf("%+v accepted", m)
		}
	}
	if _, err := tr.skipped(skip(1, 105, 103)); err == nil {
		t.Fatal("reversed skipped range accepted")
	}
}

func TestEpochArithmetic(t *testing.T) {
	if b, ok := epochBase(1); !ok || b != 0 {
		t.Fatalf("base(1) = %d", b)
	}
	if b, ok := epochBase(3); !ok || b != e3 {
		t.Fatalf("base(3) = %d", b)
	}
	if _, ok := epochBase(0); ok {
		t.Fatal("base(0) accepted")
	}
	if _, ok := epochBase(1<<24 + 1); ok {
		t.Fatal("overflowing base accepted")
	}
	if epochOf(e2) != 2 || epochOf(e2-1) != 1 || epochOf(e2+1) != 2 || epochOf(0) != 1 {
		t.Fatal("epochOf")
	}
	if inEpoch(e2, 2) || !inEpoch(e2+1, 2) || inEpoch(e3, 2) || !inEpoch(e3-1, 2) {
		t.Fatal("inEpoch")
	}
}
