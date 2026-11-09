package services

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mosub "github.com/Fabric-Labs/polyester-sdk-go/marketoverview"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// promptClose bounds a Close made from a callback: it must not wait for the
// worker that is running the callback.
const promptClose = time.Second

func closeScheduleSeeds(t *testing.T) []uint64 {
	t.Helper()
	if only := os.Getenv("POLYESTER_INTERLEAVING_SEED"); only != "" {
		seed, err := strconv.ParseUint(only, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return []uint64{seed}
	}
	count := uint64(16)
	if raw := os.Getenv("POLYESTER_INTERLEAVING_SEEDS"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		count = parsed
	}
	seeds := make([]uint64, count)
	for i := range seeds {
		seeds[i] = uint64(i)
	}
	return seeds
}

// POLY-9018 parity: closing a managed market-overview subscription from its
// own OnEvent stops delivery and returns at once instead of waiting for the
// stream worker that runs the callback.
//
// Randomized per seed: how many live publications arrive in one websocket
// message, which one's callback closes the subscription, and whether another
// goroutine also closes it, after which delay. Replay one schedule with
// POLYESTER_INTERLEAVING_SEED=<seed>.
func TestClosingManagedOverviewFromItsOwnCallbackReturnsAtOnce(t *testing.T) {
	for _, seed := range closeScheduleSeeds(t) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) { runOverviewCloseSchedule(t, seed) })
	}
}

func runOverviewCloseSchedule(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 0))
	closeAt := int64(2 + rng.IntN(3))
	last := closeAt + int64(rng.IntN(3))
	external := rng.IntN(2) == 0
	delay := []time.Duration{0, 100 * time.Microsecond, time.Millisecond, 5 * time.Millisecond}[rng.IntN(4)]

	svc, send, released, _ := overviewTicketService(t)
	var (
		mu          sync.Mutex
		tick        int
		delivered   []int64
		startedAt   []int
		closedAt    = -1
		handle      atomic.Pointer[mosub.Subscription]
		callbackRan = make(chan time.Duration, 1)
	)
	closeAndRecord := func() {
		handle.Load().Close()
		mu.Lock()
		if closedAt < 0 {
			closedAt = tick
		}
		tick++
		mu.Unlock()
	}
	sub, err := svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func(rows []models.MarketOverviewEntry) {
			price := rows[0].LastPrice.Ticks()
			mu.Lock()
			delivered = append(delivered, price)
			startedAt = append(startedAt, tick)
			tick++
			mu.Unlock()
			if price != closeAt || handle.Load() == nil {
				return
			}
			started := time.Now()
			closeAndRecord()
			callbackRan <- time.Since(started)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Store(sub)

	frames := []byte{}
	for price := int64(2); price <= last; price++ {
		frames = append(frames, overviewTicketFrame(price)...)
	}
	send(frames)
	externalDone := make(chan struct{})
	if external {
		go func() {
			time.Sleep(delay)
			closeAndRecord()
			close(externalDone)
		}()
	} else {
		close(externalDone)
	}

	waitCallback := func(took time.Duration) {
		if took > promptClose {
			t.Fatalf("seed %d: Close from OnEvent took %v", seed, took)
		}
	}
	if external {
		select {
		case took := <-callbackRan:
			waitCallback(took)
		case <-externalDone:
			// The other goroutine closed first; the callback may never reach closeAt.
		case <-time.After(5 * time.Second):
			t.Fatalf("seed %d: neither close returned", seed)
		}
	} else {
		select {
		case took := <-callbackRan:
			waitCallback(took)
		case <-time.After(5 * time.Second):
			t.Fatalf("seed %d: Close from OnEvent blocked the stream worker", seed)
		}
	}
	select {
	case <-externalDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("seed %d: Close from another goroutine hung", seed)
	}
	select {
	case <-released:
	case <-time.After(10 * time.Second):
		t.Fatalf("seed %d: the socket was not released", seed)
	}
	finalClose := make(chan struct{})
	go func() { sub.Close(); close(finalClose) }()
	select {
	case <-finalClose:
	case <-time.After(5 * time.Second):
		t.Fatalf("seed %d: a repeated Close hung", seed)
	}

	mu.Lock()
	defer mu.Unlock()
	if closedAt < 0 {
		t.Fatalf("seed %d: nothing closed the subscription", seed)
	}
	for i, at := range startedAt {
		if at > closedAt {
			t.Fatalf("seed %d: price %d delivered after Close returned (%v)", seed, delivered[i], delivered)
		}
	}
	for i, price := range delivered {
		if price != int64(i+1) {
			t.Fatalf("seed %d: deliveries out of order: %v", seed, delivered)
		}
	}
	if !external && delivered[len(delivered)-1] != closeAt {
		t.Fatalf("seed %d: delivered %v, want 1..%d", seed, delivered, closeAt)
	}
}
