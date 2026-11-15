package marketstream

import (
	"fmt"
	"math/bits"
)

// Trade IDs encode their book epoch: epoch e numbers its trades from
// epochBase(e)+1, where epochBase(e) = (e-1) * 2^40, so IDs ascend across
// epochs and epochBase(e) itself means "epoch e, before its first trade".
const epochBits = 40

// epochBase is (e-1) << 40; false for epoch 0 or one whose base overflows.
func epochBase(epoch uint64) (uint64, bool) {
	if epoch == 0 || bits.Len64(epoch-1) > 64-epochBits {
		return 0, false
	}
	return (epoch - 1) << epochBits, true
}

// epochOf is the epoch a trade ID or epoch base belongs to.
func epochOf(id uint64) uint64 { return (id >> epochBits) + 1 }

// inEpoch reports whether id is a trade ID of epoch (not its base).
func inEpoch(id, epoch uint64) bool {
	base, ok := epochBase(epoch)
	return ok && id > base && epochOf(id) == epoch
}

// tradeTracker accounts for one pair's trades stream: every trade ID up to
// last has been delivered or reported in a gap. It survives reconnects, so a
// new subscription's baseline is reconciled against what was accounted for
// before.
type tradeTracker struct {
	symbolID uint32
	// started is set by the current subscription's baseline TradesThrough.
	started bool
	// known is set once any subscription had a baseline.
	known bool
	epoch uint64
	last  uint64
	// tailOpen: the rest of epoch was reported skipped; the next message is
	// of a later epoch.
	tailOpen bool
}

// restart forgets the subscription, keeping what was accounted for.
func (t *tradeTracker) restart() { t.started = false }

func (t *tradeTracker) gap(after, through, tail uint64, cause GapCause) *TradesGap {
	return &TradesGap{SymbolID: t.symbolID, AfterTradeID: after, ThroughTradeID: through, TailEpoch: tail, Cause: cause}
}

// advanceEpoch moves the tracker to a later epoch. Trades of the epochs in
// between that no message reported are a detected gap, up to the new
// epoch's base.
func (t *tradeTracker) advanceEpoch(epoch uint64, gaps []*TradesGap) ([]*TradesGap, error) {
	base, ok := epochBase(epoch)
	if !ok {
		return gaps, fmt.Errorf("book epoch %d out of range", epoch)
	}
	switch {
	case !t.tailOpen:
		// The old epoch ended without its tail being reported: everything
		// after last, through the new epoch's base, is unaccounted.
		gaps = append(gaps, t.gap(t.last, base, 0, GapDetected))
	case epoch > t.epoch+1:
		// The old epoch's tail was reported; the epochs between were not.
		from, _ := epochBase(t.epoch + 1)
		gaps = append(gaps, t.gap(from, base, 0, GapDetected))
	}
	t.epoch, t.tailOpen = epoch, false
	t.last = max(t.last, base)
	return gaps, nil
}

// through applies a TradesThrough: the first of a subscription is its
// baseline; later ones confirm the frontier, and one past it is a detected
// gap.
func (t *tradeTracker) through(m *tradesThroughMsg) ([]*TradesGap, error) {
	if _, ok := epochBase(m.epoch); !ok || epochOf(m.last) != m.epoch {
		return nil, fmt.Errorf("trade frontier %d outside book epoch %d", m.last, m.epoch)
	}
	var gaps []*TradesGap
	if !t.started {
		t.started = true
		if !t.known {
			t.known = true
			t.epoch, t.last, t.tailOpen = m.epoch, m.last, false
			return nil, nil
		}
		// A new subscription: everything between what was accounted for
		// and its baseline is missing, across epochs if they changed.
		if m.last > t.last {
			gaps = append(gaps, t.gap(t.last, m.last, 0, GapReconnect))
			t.last = m.last
		}
		if m.epoch > t.epoch {
			t.epoch, t.tailOpen = m.epoch, false
		}
		return gaps, nil
	}
	switch {
	case m.epoch < t.epoch:
		return nil, nil
	case m.epoch > t.epoch:
		var err error
		if gaps, err = t.advanceEpoch(m.epoch, gaps); err != nil {
			return nil, err
		}
	case t.tailOpen:
		// A frontier of an epoch whose tail was reported skipped adds
		// nothing.
		return nil, nil
	}
	if m.last > t.last {
		gaps = append(gaps, t.gap(t.last, m.last, 0, GapDetected))
		t.last = m.last
	}
	return gaps, nil
}

// skipped applies a TradesSkipped.
func (t *tradeTracker) skipped(m *tradesSkippedMsg) ([]*TradesGap, error) {
	if !t.started {
		return nil, fmt.Errorf("trades skipped before the subscription's baseline")
	}
	if !inEpoch(m.first, m.epoch) || (m.last != 0 && (!inEpoch(m.last, m.epoch) || m.last < m.first)) {
		return nil, fmt.Errorf("skipped trades %d..%d outside book epoch %d", m.first, m.last, m.epoch)
	}
	var gaps []*TradesGap
	switch {
	case m.epoch < t.epoch:
		return nil, nil
	case m.epoch > t.epoch:
		var err error
		if gaps, err = t.advanceEpoch(m.epoch, gaps); err != nil {
			return nil, err
		}
	case t.tailOpen:
		return nil, nil
	}
	if m.first > t.last+1 {
		gaps = append(gaps, t.gap(t.last, m.first-1, 0, GapDetected))
		t.last = m.first - 1
	}
	if m.last == 0 {
		gaps = append(gaps, t.gap(t.last, 0, m.epoch, GapSkipped))
		t.tailOpen = true
		return gaps, nil
	}
	if m.last > t.last {
		gaps = append(gaps, t.gap(t.last, m.last, 0, GapSkipped))
		t.last = m.last
	}
	return gaps, nil
}

// trades applies a Trades message: it returns the fills not yet accounted
// for, and the gaps before them.
func (t *tradeTracker) trades(m *tradesMsg) ([]fillMsg, []*TradesGap, error) {
	if !t.started {
		return nil, nil, fmt.Errorf("trades before the subscription's baseline")
	}
	if len(m.fills) == 0 {
		return nil, nil, fmt.Errorf("trades message without trades")
	}
	prev := uint64(0)
	for _, f := range m.fills {
		if !inEpoch(f.tradeID, m.epoch) {
			return nil, nil, fmt.Errorf("trade %d outside book epoch %d", f.tradeID, m.epoch)
		}
		if f.tradeID <= prev {
			return nil, nil, fmt.Errorf("trade %d after trade %d", f.tradeID, prev)
		}
		prev = f.tradeID
	}
	var gaps []*TradesGap
	switch {
	case m.epoch < t.epoch:
		return nil, nil, nil
	case m.epoch > t.epoch:
		var err error
		if gaps, err = t.advanceEpoch(m.epoch, gaps); err != nil {
			return nil, nil, err
		}
	case t.tailOpen:
		// The rest of this epoch was reported skipped, so its recovery
		// covers these trades.
		return nil, nil, nil
	}
	fresh := m.fills[:0:0]
	for _, f := range m.fills {
		if f.tradeID <= t.last {
			continue // already delivered or reported
		}
		if f.tradeID > t.last+1 {
			gaps = append(gaps, t.gap(t.last, f.tradeID-1, 0, GapDetected))
		}
		fresh = append(fresh, f)
		t.last = f.tradeID
	}
	return fresh, gaps, nil
}
