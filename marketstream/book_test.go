package marketstream

import (
	"errors"
	"reflect"
	"testing"
)

func snap(stream Stream, epoch, updateID uint64, bids, asks []levelMsg) *depthSnapshotMsg {
	window := uint16(1000)
	if stream == StreamDepth20 {
		window = 20
	}
	return &depthSnapshotMsg{symbolID: 7, stream: uint8(stream), epoch: epoch, updateID: updateID, levelsPerSide: window, bids: bids, asks: asks}
}

func upd(epoch, first, last uint64, bids, asks []levelMsg) *depthUpdateMsg {
	return &depthUpdateMsg{symbolID: 7, epoch: epoch, first: first, last: last, bids: bids, asks: asks}
}

func lvls(pairs ...int64) []levelMsg {
	out := []levelMsg{}
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, lv(pairs[i], pairs[i+1]))
	}
	return out
}

func wantBook(t *testing.T, b *book, updateID uint64, bids, asks []levelMsg) {
	t.Helper()
	if b.updateID != updateID || !reflect.DeepEqual(append([]levelMsg{}, b.bids...), bids) || !reflect.DeepEqual(append([]levelMsg{}, b.asks...), asks) {
		t.Fatalf("book at %d bids %v asks %v; want at %d bids %v asks %v", b.updateID, b.bids, b.asks, updateID, bids, asks)
	}
}

func TestDepthSnapshotThenExactBaseUpdates(t *testing.T) {
	b := &book{stream: StreamDepth}
	if _, err := b.snapshot(snap(StreamDepth, 1, 10, lvls(100, 5, 99, 3), lvls(101, 2, 103, 4))); err != nil {
		t.Fatal(err)
	}
	// Set, insert and delete on both sides.
	if err := b.update(upd(1, 11, 14, lvls(100, 6, 98, 1, 99, 0), lvls(102, 7, 101, 0))); err != nil {
		t.Fatal(err)
	}
	wantBook(t, b, 14, lvls(100, 6, 98, 1), lvls(102, 7, 103, 4))
	// An empty update keeps the next one contiguous.
	if err := b.update(upd(1, 15, 20, nil, nil)); err != nil {
		t.Fatal(err)
	}
	wantBook(t, b, 20, lvls(100, 6, 98, 1), lvls(102, 7, 103, 4))
	// A replacement snapshot replaces the book, whatever it held, an older
	// one included.
	if changed, err := b.snapshot(snap(StreamDepth, 1, 15, lvls(97, 1), nil)); err != nil || !changed {
		t.Fatalf("older snapshot: changed %t, %v", changed, err)
	}
	wantBook(t, b, 15, lvls(97, 1), lvls())
}

func TestDepthUpdatesTheBookCannotApply(t *testing.T) {
	b := &book{stream: StreamDepth}
	var gap *errBookGap
	if err := b.update(upd(1, 11, 11, nil, nil)); !errors.As(err, &gap) {
		t.Fatalf("update before a snapshot: %v", err)
	}
	if _, err := b.snapshot(snap(StreamDepth, 1, 10, lvls(100, 5), lvls(101, 2))); err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]*depthUpdateMsg{
		"after a gap":      upd(1, 12, 13, nil, nil),
		"inside a range":   upd(1, 10, 13, nil, nil),
		"already applied":  upd(1, 9, 10, nil, nil),
		"of another epoch": upd(2, 11, 11, nil, nil),
	} {
		if err := b.update(u); !errors.As(err, &gap) {
			t.Fatalf("%s: %v, want a book gap", name, err)
		}
	}
	wantBook(t, b, 10, lvls(100, 5), lvls(101, 2))
	for name, u := range map[string]*depthUpdateMsg{
		"reversed range":     upd(1, 11, 10, nil, nil),
		"negative quantity":  upd(1, 11, 11, lvls(100, -1), nil),
		"price not positive": upd(1, 11, 11, nil, lvls(0, 1)),
	} {
		if err := b.update(u); err == nil || errors.As(err, &gap) {
			t.Fatalf("%s: %v, want a malformed update", name, err)
		}
	}
}

func TestDepthWindowTruncatesEachSide(t *testing.T) {
	b := &book{stream: StreamDepth}
	window := snap(StreamDepth, 1, 1, lvls(100, 1, 99, 1, 98, 1), lvls(101, 1, 102, 1, 103, 1))
	window.levelsPerSide = 3
	if _, err := b.snapshot(window); err != nil {
		t.Fatal(err)
	}
	// Better levels push the worst out of a three-level window; the server
	// sends no delete for them.
	if err := b.update(upd(1, 2, 2, lvls(105, 1), lvls(100, 9))); err != nil {
		t.Fatal(err)
	}
	wantBook(t, b, 2, lvls(105, 1, 100, 1, 99, 1), lvls(100, 9, 101, 1, 102, 1))
}

func TestDepth20KeepsTheNewestSnapshot(t *testing.T) {
	b := &book{stream: StreamDepth20}
	changed, err := b.snapshot(snap(StreamDepth20, 2, 50, lvls(100, 1), nil))
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	for _, s := range []*depthSnapshotMsg{
		snap(StreamDepth20, 2, 50, lvls(1, 1), nil), // a re-send
		snap(StreamDepth20, 2, 49, lvls(2, 1), nil), // older
		snap(StreamDepth20, 1, 90, lvls(3, 1), nil), // older epoch
	} {
		if changed, err := b.snapshot(s); err != nil || changed {
			t.Fatalf("%+v replaced the book", s)
		}
	}
	if changed, err := b.snapshot(snap(StreamDepth20, 3, 1, lvls(4, 1), nil)); err != nil || !changed {
		t.Fatal("a newer epoch did not replace the book")
	}
	wantBook(t, b, 1, lvls(4, 1), lvls())
}

func TestSnapshotLevelsMustBeOrderedAndPositive(t *testing.T) {
	for name, s := range map[string]*depthSnapshotMsg{
		"bids ascending":  snap(StreamDepth, 1, 1, lvls(99, 1, 100, 1), nil),
		"asks descending": snap(StreamDepth, 1, 1, nil, lvls(101, 1, 100, 1)),
		"repeated price":  snap(StreamDepth, 1, 1, lvls(99, 1, 99, 2), nil),
		"zero quantity":   snap(StreamDepth, 1, 1, lvls(99, 0), nil),
		"zero price":      snap(StreamDepth, 1, 1, nil, lvls(0, 1)),
		"no window":       {symbolID: 7, stream: uint8(StreamDepth), epoch: 1, updateID: 1},
		"past its window": {symbolID: 7, stream: uint8(StreamDepth), epoch: 1, updateID: 1, levelsPerSide: 1, bids: lvls(100, 1, 99, 1)},
	} {
		b := &book{stream: StreamDepth}
		if _, err := b.snapshot(s); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
