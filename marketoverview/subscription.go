package marketoverview

import (
	"context"
	"sync"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// Subscription is a merged market-overview stream.
type Subscription struct {
	updates   chan []models.MarketOverviewEntry
	stream    *realtime.SnapshotThenStream[models.MarketOverviewList, models.MarketOverviewList]
	closeOnce sync.Once
	closed    bool
	mu        sync.Mutex
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
		s.stream.Close()
		close(s.updates)
	})
}

// Enqueue publishes the current merged overview rows.
func (s *Subscription) Enqueue(rows []models.MarketOverviewEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.updates <- rows:
	default:
	}
}
