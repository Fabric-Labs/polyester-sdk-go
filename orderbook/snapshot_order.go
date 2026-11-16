package orderbook

import (
	"cmp"
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// CompareSnapshots orders two Orderbook.Get snapshots of one pair by
// (BookEpoch, BookSeq): it returns -1 when a is older than b, 0 when both are
// the same book state, and +1 when a is newer. A higher epoch is newer whatever
// its sequence, since a new epoch may restart the sequence; sequences are
// compared only within one epoch. Keep the newest snapshot and drop one whose
// epoch is older than the newest seen. A snapshot without a BookEpoch, such as
// a managed subscription's view, or with a BookEpoch or BookSeq that is not a
// decimal uint64, is a *errors.ValidationError: it cannot be placed.
func CompareSnapshots(a, b models.OrderbookData) (int, error) {
	aEpoch, aSeq, err := snapshotPosition(a, "a")
	if err != nil {
		return 0, err
	}
	bEpoch, bSeq, err := snapshotPosition(b, "b")
	if err != nil {
		return 0, err
	}
	if c := cmp.Compare(aEpoch, bEpoch); c != 0 {
		return c, nil
	}
	return cmp.Compare(aSeq, bSeq), nil
}

func snapshotPosition(data models.OrderbookData, name string) (epoch, seq uint64, err error) {
	if data.BookEpoch == "" {
		return 0, 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook snapshot %s has no book_epoch; only Orderbook.Get snapshots can be ordered", name)}
	}
	epoch, err = strconv.ParseUint(data.BookEpoch, 10, 64)
	if err != nil {
		return 0, 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook snapshot %s book_epoch %q is not a decimal uint64", name, data.BookEpoch)}
	}
	seq, err = strconv.ParseUint(data.BookSeq, 10, 64)
	if err != nil {
		return 0, 0, &errors.ValidationError{Msg: fmt.Sprintf("orderbook snapshot %s book_seq %q is not a decimal uint64", name, data.BookSeq)}
	}
	return epoch, seq, nil
}
