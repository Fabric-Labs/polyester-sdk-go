package realtime

import (
	"sync"
	"sync/atomic"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// Subscription delivers decoded realtime publications.
//
// Delivery contract: the queue is bounded. If the consumer cannot keep up,
// enqueue fails the subscription with QueueOverflowError instead of silently
// dropping updates.
//
// After a transport reconnect the subscription is re-established without a
// server-side resume cursor. Resubscribes()/TakeResubscribed() signal that gap:
// publications may have been lost between disconnect and the new subscribe.
//
// Close cancels the subscription context and waits until the run loop has
// fully stopped (websocket teardown included).
type Subscription[T any] struct {
	ch           chan T
	done         chan struct{}
	closeOnce    sync.Once
	finishOnce   sync.Once
	closed       bool
	mu           sync.Mutex
	cancel       func()
	err          error
	resubscribes atomic.Uint64
	resubscribed atomic.Bool
	untrack      func()
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

// Close cancels the subscription and waits for the run loop to exit.
func (s *Subscription[T]) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		cancel := s.cancel
		untrack := s.untrack
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if untrack != nil {
			untrack()
		}
	})
	<-s.done
}

// markFinished is called by the subscription goroutine when it exits.
func (s *Subscription[T]) markFinished() {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
	})
}

// Resubscribes returns how many times this subscription successfully
// reconnected after the initial connect. A non-zero value means the stream may
// have gaps relative to a continuous subscription (possible data loss).
func (s *Subscription[T]) Resubscribes() uint64 {
	return s.resubscribes.Load()
}

// TakeResubscribed reports whether a reconnect/resubscribe happened since the
// last call and clears the latch. Callers can poll this to refresh REST state
// after a gap. The initial connect does not set the latch.
func (s *Subscription[T]) TakeResubscribed() bool {
	return s.resubscribed.Swap(false)
}

// noteHandshakeReady records a successful Centrifugo connect/subscribe.
// first=true is the initial handshake; subsequent calls signal a gap/resubscribe.
func (s *Subscription[T]) noteHandshakeReady(first bool) {
	if first {
		return
	}
	s.resubscribes.Add(1)
	s.resubscribed.Store(true)
}

func (s *Subscription[T]) failLocked(err error) {
	if s.err == nil {
		s.err = err
	}
	s.closed = true
}

func (s *Subscription[T]) fail(err error) {
	s.mu.Lock()
	s.failLocked(err)
	s.mu.Unlock()
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
		return false
	}
}
