package marketoverview

import (
	"context"
	"sync"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// Subscription is a merged market-overview stream.
//
// Delivery contract matches realtime.Subscription: a full consumer queue fails
// the subscription with QueueOverflowError instead of silently dropping rows.
type Subscription struct {
	updates   chan []models.MarketOverviewEntry
	stream    *realtime.SnapshotThenStream[models.MarketOverviewList, models.MarketOverviewList]
	closeOnce sync.Once
	closed    bool
	mu        sync.Mutex
	err       error
	onError   func(error)
}

// NewSubscription creates a managed market overview subscription.
func NewSubscription(stream *realtime.SnapshotThenStream[models.MarketOverviewList, models.MarketOverviewList]) *Subscription {
	return &Subscription{
		updates: make(chan []models.MarketOverviewEntry, 50),
		stream:  stream,
	}
}

// Updates returns merged overview rows.
func (s *Subscription) Updates() <-chan []models.MarketOverviewEntry {
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
		callErrorCallback(callback, err)
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

// Enqueue publishes the current merged overview rows.
// Returns false and fails the subscription when the queue is full.
func (s *Subscription) Enqueue(rows []models.MarketOverviewEntry) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	select {
	case s.updates <- rows:
		s.mu.Unlock()
		return true
	default:
		if s.err == nil {
			s.err = &sdkerrors.QueueOverflowError{
				Msg: "market overview subscription queue full; consumer too slow",
			}
		}
		err := s.err
		callback := s.onError
		s.closed = true
		s.mu.Unlock()
		callErrorCallback(callback, err)
		s.Close()
		return false
	}
}

func callErrorCallback(callback func(error), err error) {
	if callback == nil || err == nil {
		return
	}
	defer func() { _ = recover() }()
	callback(err)
}
