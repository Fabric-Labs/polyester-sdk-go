package marketstream

import (
	"fmt"
	"math"
	"sort"
)

// OrderBook is a copy of a pair's book of the depth20 or depth stream. Bids
// are best (highest) first, asks best (lowest) first.
type OrderBook struct {
	SymbolID  uint32
	Symbol    string
	Stream    Stream
	BookEpoch uint64
	// UpdateID is the book update the book shows.
	UpdateID uint64
	// LevelsPerSide is the most levels per side the stream holds: 20 for
	// depth20, the live window for depth.
	LevelsPerSide int
	Bids          []Level
	Asks          []Level
	// Stale is set while the book may not be current: the pair's feed is
	// stale, or the connection is being re-established and the book waits
	// for its next snapshot. The levels are then the last state known.
	Stale bool
}

// book is a pair's book of one depth stream, in wire units: bids descending
// by price, asks ascending.
type book struct {
	stream Stream
	// synced: the book holds a snapshot of the current subscription and has
	// followed every update since.
	synced          bool
	has             bool
	epoch, updateID uint64
	// behind is set when this connection's snapshot was older than the
	// book: the book keeps its state, stale, and resumes with the first
	// update at its base; updates up to it are skipped.
	behind bool
	// window is the snapshot's levelsPerSide: each side is truncated to it
	// after every update.
	window     int
	bids, asks []levelMsg
}

// validLevels checks a snapshot side: positive prices and quantities,
// strictly ordered best first.
func validLevels(levels []levelMsg, descending bool) error {
	for i, l := range levels {
		if l.price <= 0 || l.qty <= 0 {
			return fmt.Errorf("level %d has price %d and quantity %d", i, l.price, l.qty)
		}
		if i > 0 {
			prev := levels[i-1].price
			if (descending && l.price >= prev) || (!descending && l.price <= prev) {
				return fmt.Errorf("level %d at price %d is out of order", i, l.price)
			}
		}
	}
	return nil
}

// snapshot replaces the book. The book keeps its highest (epoch, update) for
// its whole life, across connections: an older snapshot changes nothing and
// leaves the book as it is, current or waiting; an equal one confirms it (on
// depth20, a re-send that changes nothing while the book is current); a newer
// one, a higher epoch included, replaces it. It reports whether the book
// changed or was confirmed.
func (b *book) snapshot(m *depthSnapshotMsg) (bool, error) {
	if err := validLevels(m.bids, true); err != nil {
		return false, fmt.Errorf("snapshot bids: %w", err)
	}
	if err := validLevels(m.asks, false); err != nil {
		return false, fmt.Errorf("snapshot asks: %w", err)
	}
	if m.levelsPerSide == 0 || len(m.bids) > int(m.levelsPerSide) || len(m.asks) > int(m.levelsPerSide) {
		return false, fmt.Errorf("snapshot of %d bids and %d asks for %d levels per side", len(m.bids), len(m.asks), m.levelsPerSide)
	}
	if b.has {
		older := m.epoch < b.epoch || m.epoch == b.epoch && m.updateID < b.updateID
		equal := m.epoch == b.epoch && m.updateID == b.updateID
		switch {
		case older:
			b.behind = !b.synced
			return false, nil
		case equal && b.synced && b.stream == StreamDepth20:
			return false, nil
		}
	}
	b.synced, b.has, b.behind = true, true, false
	b.epoch, b.updateID, b.window = m.epoch, m.updateID, int(m.levelsPerSide)
	b.bids = append(b.bids[:0], m.bids...)
	b.asks = append(b.asks[:0], m.asks...)
	return true, nil
}

// errBookGap reports an update the book cannot apply: the book is not at its
// base. The book stops following the stream until a new snapshot.
type errBookGap struct{ msg string }

func (e *errBookGap) Error() string { return e.msg }

// update applies a DepthUpdate at exactly its base, then truncates each side
// to the snapshot's levels per side. It reports false for an update skipped
// while the book waits for a replica behind it to catch up.
func (b *book) update(m *depthUpdateMsg) (bool, error) {
	if m.last < m.first {
		return false, fmt.Errorf("update covers %d..%d", m.first, m.last)
	}
	for _, side := range [][]levelMsg{m.bids, m.asks} {
		for i, l := range side {
			if l.price <= 0 || l.qty < 0 {
				return false, fmt.Errorf("update level %d has price %d and quantity %d", i, l.price, l.qty)
			}
		}
	}
	if b.behind && m.epoch == b.epoch {
		switch {
		case m.last <= b.updateID:
			return false, nil // the replica catching up to the book
		case b.updateID < math.MaxUint64 && m.first == b.updateID+1:
			b.synced, b.behind = true, false // the book's own base: it resumes
		}
	}
	switch {
	case !b.synced:
		return false, &errBookGap{msg: fmt.Sprintf("depth update %d..%d before a snapshot", m.first, m.last)}
	case m.epoch != b.epoch:
		return false, &errBookGap{msg: fmt.Sprintf("depth update of book epoch %d on a book of epoch %d", m.epoch, b.epoch)}
	case b.updateID == math.MaxUint64 || m.first != b.updateID+1:
		return false, &errBookGap{msg: fmt.Sprintf("depth update %d..%d on a book at %d", m.first, m.last, b.updateID)}
	}
	b.bids = applyLevels(b.bids, m.bids, true)
	b.asks = applyLevels(b.asks, m.asks, false)
	b.bids = b.bids[:min(len(b.bids), b.window)]
	b.asks = b.asks[:min(len(b.asks), b.window)]
	b.updateID = m.last
	return true, nil
}

// applyLevels sets each level of changes on side, deleting those with
// quantity 0, and keeps the side ordered best first.
func applyLevels(side, changes []levelMsg, descending bool) []levelMsg {
	for _, c := range changes {
		i := sort.Search(len(side), func(i int) bool {
			if descending {
				return side[i].price <= c.price
			}
			return side[i].price >= c.price
		})
		found := i < len(side) && side[i].price == c.price
		switch {
		case c.qty == 0 && found:
			side = append(side[:i], side[i+1:]...)
		case c.qty == 0:
		case found:
			side[i].qty = c.qty
		default:
			side = append(side, levelMsg{})
			copy(side[i+1:], side[i:])
			side[i] = c
		}
	}
	return side
}
