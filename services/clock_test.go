package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock is a serviceClock whose time moves only when a service waits:
// each Wait advances it by the wait and returns at once. A retry budget or a
// polling window then counts the waits alone, however long the requests take
// on a loaded machine.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Wait(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.waits = append(c.waits, d)
	return nil
}

// WithDeadline leaves ctx unbounded: the clock moves only in Wait, which the
// services check against their deadline before they wait.
func (c *fakeClock) WithDeadline(ctx context.Context, _ time.Time) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}

// Waits returns the waits so far, in order.
func (c *fakeClock) Waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

// useFakeClock makes every service wait read a fakeClock that starts at the
// wall clock's now, until the test ends. Tests that use it must not run in
// parallel.
func useFakeClock(t *testing.T) *fakeClock {
	t.Helper()
	clock := &fakeClock{now: time.Now()}
	saved := waitClock
	waitClock = clock
	t.Cleanup(func() { waitClock = saved })
	return clock
}
