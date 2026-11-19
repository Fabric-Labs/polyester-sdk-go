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

// simClock is a serviceClock whose time moves only when the code waits or a
// test advances it, and whose deadlines fire, cancelling their context, as it
// reaches them. A wait bounded by a deadline then ends exactly when the
// schedule says, however slowly a loaded machine runs.
type simClock struct {
	mu        sync.Mutex
	now       time.Time
	deadlines []simDeadline
}

type simDeadline struct {
	at     time.Time
	cancel context.CancelFunc
}

func (c *simClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock by d and fires every deadline it reaches. It
// reports whether one fired.
func (c *simClock) Advance(d time.Duration) bool {
	c.mu.Lock()
	c.now = c.now.Add(d)
	fired := c.fireLocked()
	c.mu.Unlock()
	for _, cancel := range fired {
		cancel()
	}
	return len(fired) > 0
}

// AdvanceToNextDeadline moves the clock to the earliest pending deadline and
// fires it: the time a read that never answers takes. It reports whether
// there was one.
func (c *simClock) AdvanceToNextDeadline() bool {
	c.mu.Lock()
	if len(c.deadlines) == 0 {
		c.mu.Unlock()
		return false
	}
	next := c.deadlines[0].at
	for _, deadline := range c.deadlines[1:] {
		if deadline.at.Before(next) {
			next = deadline.at
		}
	}
	if next.After(c.now) {
		c.now = next
	}
	fired := c.fireLocked()
	c.mu.Unlock()
	for _, cancel := range fired {
		cancel()
	}
	return true
}

func (c *simClock) fireLocked() []context.CancelFunc {
	var fired []context.CancelFunc
	pending := c.deadlines[:0]
	for _, deadline := range c.deadlines {
		if deadline.at.After(c.now) {
			pending = append(pending, deadline)
			continue
		}
		fired = append(fired, deadline.cancel)
	}
	c.deadlines = pending
	return fired
}

// Wait moves the clock by d; a deadline it reaches ends ctx, and Wait then
// returns ctx.Err(), as a wall-clock wait does when ctx ends first.
func (c *simClock) Wait(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.Advance(d)
	return ctx.Err()
}

func (c *simClock) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	bounded, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.deadlines = append(c.deadlines, simDeadline{at: deadline, cancel: cancel})
	fired := c.fireLocked()
	c.mu.Unlock()
	for _, fire := range fired {
		fire()
	}
	return bounded, cancel
}

// useSimClock makes every service wait read a simClock that starts at the
// wall clock's now, until the test ends. Tests that use it must not run in
// parallel.
func useSimClock(t *testing.T) *simClock {
	t.Helper()
	clock := &simClock{now: time.Now()}
	saved := waitClock
	waitClock = clock
	t.Cleanup(func() { waitClock = saved })
	return clock
}
