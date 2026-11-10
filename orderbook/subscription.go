package orderbook

import (
	"context"
	"sync"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/callbackmark"
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
	setBucket func(int)
	closeOnce sync.Once
	done      chan struct{}
	closed    bool
	err       error
	onError   func(error)
}

// NewSubscription wires snapshot-then-stream state for an orderbook channel.
func NewSubscription(
	stream *realtime.SnapshotThenStream[models.OrderbookData, models.OrderBookDeltaUpdate],
	emit func(),
	setBucket ...func(int),
) *Subscription {
	var setBucketFn func(int)
	if len(setBucket) > 0 {
		setBucketFn = setBucket[0]
	}
	sub := &Subscription{
		updates:   make(chan models.OrderbookData, 200),
		done:      make(chan struct{}),
		stream:    stream,
		emit:      emit,
		setBucket: setBucketFn,
	}
	if stream != nil {
		go func() {
			select {
			case <-stream.Done():
				sub.closeUpdates()
			case <-sub.done:
			}
		}()
	}
	return sub
}

// Updates returns merged orderbook snapshots.
func (s *Subscription) Updates() <-chan models.OrderbookData {
	return s.updates
}

// Err returns the terminal subscription error, if any.
func (s *Subscription) Err() error {
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err == nil && s.stream != nil {
		return s.stream.Err()
	}
	return err
}

// SetOnError installs a callback for managed snapshot/stream and queue errors.
func (s *Subscription) SetOnError(callback func(error)) {
	s.mu.Lock()
	s.onError = callback
	err := s.err
	s.mu.Unlock()
	if s.stream != nil {
		s.stream.SetOnError(callback)
	}
	if err != nil {
		s.runCallback(func() { callErrorCallback(callback, err) })
	}
}

// SetBucket changes the active price bucket.
func (s *Subscription) SetBucket(bucket string) {
	ticks := ParseBucketTicks(bucket)
	s.mu.Lock()
	s.bucket = ticks
	setBucket := s.setBucket
	s.mu.Unlock()
	if setBucket != nil {
		setBucket(ticks)
	}
	// Before a snapshot there is no book to render: a default view would read as
	// a real empty book.
	if s.emit != nil && (s.stream == nil || s.stream.IsReady()) {
		s.runCallback(s.emit)
	}
}

// RefreshSnapshot refetches the REST snapshot.
func (s *Subscription) RefreshSnapshot(ctx context.Context) error {
	return s.stream.RefreshSnapshot(ctx)
}

// Close stops the subscription.
func (s *Subscription) Close() {
	s.closeUpdates()
	if s.stream != nil {
		s.stream.Close()
	}
}

func (s *Subscription) closeUpdates() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed = true
		close(s.updates)
		if s.done != nil {
			close(s.done)
		}
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
		s.mu.Unlock()
		s.Fail(&sdkerrors.QueueOverflowError{
			Msg: "orderbook subscription queue full; consumer too slow",
		})
		return false
	}
}

// Closed reports whether the subscription has ended, by Close or a terminal error.
func (s *Subscription) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Fail ends the subscription with a terminal error: the error is retained for
// Err, Updates closes and the error callback runs once. Later calls do nothing.
func (s *Subscription) Fail(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.err == nil {
		s.err = err
	}
	err = s.err
	callback := s.onError
	s.closed = true
	s.mu.Unlock()
	s.closeUpdates()
	if s.stream != nil {
		s.stream.Stop()
	}
	s.runCallback(func() { callErrorCallback(callback, err) })
}

// runCallback runs fn as a callback of the stream, so a Close from inside it
// signals shutdown without waiting for the stream's worker.
func (s *Subscription) runCallback(fn func()) {
	if s.stream != nil {
		callbackmark.Run(s.stream, fn)
		return
	}
	fn()
}

func callErrorCallback(callback func(error), err error) {
	if callback == nil || err == nil {
		return
	}
	defer func() { _ = recover() }()
	callback(err)
}
