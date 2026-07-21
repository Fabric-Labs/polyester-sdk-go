package realtime

import (
	"sync"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// Subscription delivers decoded realtime publications.
//
// Delivery contract: the queue is bounded. If the consumer cannot keep up,
// enqueue fails the subscription with QueueOverflowError instead of silently
// dropping updates.
type Subscription[T any] struct {
	ch     chan T
	done   chan struct{}
	close  sync.Once
	closed bool
	mu     sync.Mutex
	cancel func()
	err    error
}

func newSubscription[T any](maxQueue int, cancel func()) *Subscription[T] {
	if maxQueue <= 0 {
		maxQueue = 1000
	}
	return &Subscription[T]{
		ch:     make(chan T, maxQueue),
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

// Messages returns the publication channel. It closes when the subscription ends.
func (s *Subscription[T]) Messages() <-chan T {
	return s.ch
}

// Done signals when the subscription has fully stopped.
func (s *Subscription[T]) Done() <-chan struct{} {
	return s.done
}

// Err returns the terminal subscription error, if any.
// A non-nil QueueOverflowError means updates were not silently dropped —
// the subscription was failed instead.
func (s *Subscription[T]) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close stops the subscription.
func (s *Subscription[T]) Close() {
	s.close.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
		close(s.done)
	})
}

func (s *Subscription[T]) failLocked(err error) {
	if s.err == nil {
		s.err = err
	}
	s.closed = true
}

// enqueue delivers an item or fails the subscription on overflow.
// Returns true when the item was queued. On overflow it records
// QueueOverflowError, closes the subscription, and returns false.
func (s *Subscription[T]) enqueue(item T) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	select {
	case s.ch <- item:
		s.mu.Unlock()
		return true
	default:
		s.failLocked(&sdkerrors.QueueOverflowError{
			Msg: "realtime subscription queue full; consumer too slow",
		})
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.Close()
		return false
	}
}
