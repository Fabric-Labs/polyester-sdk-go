package orderbook

import (
	"context"
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

func TestManagedSubscriptionSurfacesStreamErrorsAndUpdatesBucket(t *testing.T) {
	want := errors.New("snapshot failed")
	stream := realtime.NewSnapshotThenStream(
		realtime.SnapshotThenStreamConfig[models.OrderbookData, models.OrderBookDeltaUpdate]{
			FetchSnapshot: func(context.Context) (models.OrderbookData, error) {
				return models.OrderbookData{}, want
			},
			ReadPublication: func(p models.OrderBookDeltaUpdate) []models.OrderBookDeltaUpdate {
				return []models.OrderBookDeltaUpdate{p}
			},
			ApplySnapshot:         func(models.OrderbookData, []models.OrderBookDeltaUpdate) {},
			ApplyLivePublications: func([]models.OrderBookDeltaUpdate) {},
		},
	)
	var bucket int
	sub := NewSubscription(stream, func() {}, func(ticks int) { bucket = ticks })
	var observed error
	sub.SetOnError(func(err error) { observed = err })

	if err := sub.RefreshSnapshot(context.Background()); !errors.Is(err, want) {
		t.Fatalf("RefreshSnapshot()=%v want %v", err, want)
	}
	if !errors.Is(sub.Err(), want) {
		t.Fatalf("Err()=%v want %v", sub.Err(), want)
	}
	if !errors.Is(observed, want) {
		t.Fatalf("OnError=%v want %v", observed, want)
	}

	sub.SetBucket("0.01")
	if bucket != 10_000 {
		t.Fatalf("bucket ticks=%d want 10000", bucket)
	}
	sub.Close()
}
