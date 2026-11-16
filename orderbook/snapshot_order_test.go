package orderbook

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func snapshotAt(epoch, seq string) models.OrderbookData {
	return models.OrderbookData{Symbol: "BTC-USDT", BookEpoch: epoch, BookSeq: seq}
}

// Snapshots order by (BookEpoch, BookSeq): a higher epoch is newer whatever its
// sequence, and sequences compare only within one epoch.
func TestCompareSnapshotsOrdersByEpochThenSequence(t *testing.T) {
	for _, tc := range []struct {
		a, b models.OrderbookData
		want int
	}{
		{snapshotAt("3", "984321"), snapshotAt("3", "984321"), 0},
		{snapshotAt("3", "984320"), snapshotAt("3", "984321"), -1},
		{snapshotAt("3", "984322"), snapshotAt("3", "984321"), 1},
		// A new epoch restarts the sequence and is still newer.
		{snapshotAt("4", "1"), snapshotAt("3", "984321"), 1},
		{snapshotAt("3", "984321"), snapshotAt("4", "1"), -1},
		// An unavailable snapshot (epoch 0) is older than any available one.
		{snapshotAt("0", "0"), snapshotAt("1", "0"), -1},
		// The whole uint64 range of both.
		{snapshotAt("18446744073709551615", "0"), snapshotAt("18446744073709551614", "18446744073709551615"), 1},
	} {
		got, err := CompareSnapshots(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("CompareSnapshots(%s/%s, %s/%s) = %d, %v; want %d", tc.a.BookEpoch, tc.a.BookSeq, tc.b.BookEpoch, tc.b.BookSeq, got, err, tc.want)
		}
	}
}

// A view without an epoch, such as a managed subscription's, or a malformed
// position cannot be ordered.
func TestCompareSnapshotsRefusesUnplaceableSnapshots(t *testing.T) {
	good := snapshotAt("1", "1")
	for _, bad := range []models.OrderbookData{
		snapshotAt("", "1"),
		snapshotAt("-1", "1"),
		snapshotAt("+1", "1"),
		snapshotAt("1.0", "1"),
		snapshotAt("18446744073709551616", "1"),
		snapshotAt("1", ""),
		snapshotAt("1", "x"),
	} {
		for _, pair := range [][2]models.OrderbookData{{bad, good}, {good, bad}} {
			_, err := CompareSnapshots(pair[0], pair[1])
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("epoch %q seq %q: err=%v, want ValidationError", bad.BookEpoch, bad.BookSeq, err)
			}
		}
	}
}
