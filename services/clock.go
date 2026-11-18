package services

import (
	"context"
	"time"
)

// serviceClock is the time the services' read retries and polling waits
// read: when the wait started, how long a wait lasts, and when a bounded
// read's deadline passes. Production uses the wall clock; a test replaces it
// so a retry budget or a polling window does not depend on how fast the
// machine runs.
type serviceClock interface {
	Now() time.Time
	// Wait returns after d, or ctx.Err() once ctx ends first.
	Wait(ctx context.Context, d time.Duration) error
	// WithDeadline bounds ctx by deadline on this clock.
	WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc)
}

// waitClock is the clock every service wait reads.
var waitClock serviceClock = wallClock{}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func (wallClock) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (wallClock) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, deadline)
}
