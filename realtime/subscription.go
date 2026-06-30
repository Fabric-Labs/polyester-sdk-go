package realtime

import "sync"

// Subscription delivers decoded realtime publications.
type Subscription[T any] struct {
	ch     chan T
	done   chan struct{}
	close  sync.Once
	closed bool
	mu     sync.Mutex
	cancel func()
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

func (s *Subscription[T]) enqueue(item T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- item:
		return true
	default:
		return false
	}
}
