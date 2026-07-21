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
	defer s.mu.Unlock()
	return s.err
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
		s.closed = true
		s.mu.Unlock()
		s.Close()
		return false
	}
}
