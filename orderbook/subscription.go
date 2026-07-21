package orderbook

import (
	"context"
	"sync"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// Subscription is a managed orderbook stream with snapshot prefetch and sequence-checked deltas.
//
// Delivery contract matches realtime.Subscription: a full consumer queue fails
// the subscription with QueueOverflowError instead of silently dropping books.
type Subscription struct {
	updates   chan models.OrderbookData
	stream    *realtime.SnapshotThenStream[models.OrderbookData, models.OrderBookDeltaUpdate]
	mu        sync.Mutex
	bucket    int
	emit      func()
	closeOnce sync.Once
	closed    bool
	err       error
}

// NewSubscription wires snapshot-then-stream state for an orderbook channel.
func NewSubscription(
	stream *realtime.SnapshotThenStream[models.OrderbookData, models.OrderBookDeltaUpdate],
	emit func(),
) *Subscription {
	return &Subscription{
		updates: make(chan models.OrderbookData, 200),
		stream:  stream,
		emit:    emit,
	}
}

// Updates returns merged orderbook snapshots.
func (s *Subscription) Updates() <-chan models.OrderbookData {
	return s.updates
}

// Err returns the terminal subscription error, if any.
func (s *Subscription) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// SetBucket changes the active price bucket.
func (s *Subscription) SetBucket(bucket string) {
	s.mu.Lock()
	s.bucket = ParseBucketTicks(bucket)
	s.mu.Unlock()
	if s.emit != nil {
		s.emit()
	}
}

// RefreshSnapshot refetches the REST snapshot.
func (s *Subscription) RefreshSnapshot(ctx context.Context) error {
	return s.stream.RefreshSnapshot(ctx)
}

// Close stops the subscription.
func (s *Subscription) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if s.stream != nil {
			s.stream.Close()
		}
		close(s.updates)
	})
}

// Enqueue adds an orderbook snapshot to the consumer channel.
// Returns false and fails the subscription when the queue is full.
func (s *Subscription) Enqueue(data models.OrderbookData) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	select {
	case s.updates <- data:
		s.mu.Unlock()
		return true
	default:
		if s.err == nil {
			s.err = &sdkerrors.QueueOverflowError{
				Msg: "orderbook subscription queue full; consumer too slow",
			}
		}
		s.closed = true
		s.mu.Unlock()
		s.Close()
		return false
	}
}
