package realtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestRefreshSnapshotFiresOnSnapshotRefresh(t *testing.T) {
	events := make([]string, 0, 1)
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		Client:  nil,
		Channel: "public:test",
		Decode:  func([]byte) (int, error) { return 0, nil },
		FetchSnapshot: func(context.Context) (string, error) {
			return "snap", nil
		},
		ReadPublication:       func(p int) []int { return []int{p} },
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		OnSnapshotRefresh:     func() { events = append(events, "snapshot_refresh") },
	})

	if err := sts.RefreshSnapshot(context.Background()); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}
	if len(events) != 1 || events[0] != "snapshot_refresh" {
		t.Fatalf("events = %#v, want [snapshot_refresh]", events)
	}
	if !sts.IsReady() {
		t.Fatal("expected ready after RefreshSnapshot")
	}
}

func TestRefreshSnapshotAtomicallyDrainsBeforeReady(t *testing.T) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var mu sync.Mutex
	var live []int
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot:   func(context.Context) (string, error) { return "snap", nil },
		ReadPublication: func(p int) []int { return []int{p} },
		ApplySnapshot: func(string, []int) {
			close(applyStarted)
			<-releaseApply
		},
		ApplyLivePublications: func(items []int) {
			mu.Lock()
			live = append(live, items...)
			mu.Unlock()
		},
		Decode: func([]byte) (int, error) { return 0, nil },
	})
	done := make(chan error, 1)
	go func() { done <- sts.RefreshSnapshot(context.Background()) }()
	<-applyStarted
	sts.handlePublication(7)
	close(releaseApply)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sts.IsReady() || len(live) != 1 || live[0] != 7 {
		t.Fatalf("ready=%v live=%v", sts.IsReady(), live)
	}
}

func TestRequestedRefreshIsSingleFlightAndBounded(t *testing.T) {
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var calls atomic.Int32
	release := make(chan struct{})
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			calls.Add(1)
			current := concurrent.Add(1)
			for current > maxConcurrent.Load() && !maxConcurrent.CompareAndSwap(maxConcurrent.Load(), current) {
			}
			<-release
			concurrent.Add(-1)
			return "snap", nil
		},
		ReadPublication:       func(p int) []int { return []int{p} },
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		Decode:                func([]byte) (int, error) { return 0, nil },
	})
	for range 20 {
		sts.RequestSnapshotRefresh(context.Background())
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if maxConcurrent.Load() != 1 || calls.Load() > 2 {
		t.Fatalf("max concurrent=%d calls=%d", maxConcurrent.Load(), calls.Load())
	}
	sts.Close()
}

func TestSnapshotRecoveryBufferOverflowFailsClosed(t *testing.T) {
	var observed error
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		Decode:                func([]byte) (int, error) { return 0, nil },
		FetchSnapshot:         func(context.Context) (string, error) { return "snap", nil },
		ReadPublication:       func(p int) []int { return []int{p} },
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		MaxBuffered:           1,
		OnError:               func(err error) { observed = err },
	})
	sts.handlePublication(1)
	sts.handlePublication(2)
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(sts.Err(), &overflow) {
		t.Fatalf("want QueueOverflowError, got %T: %v", sts.Err(), sts.Err())
	}
	if !errors.As(observed, &overflow) {
		t.Fatalf("OnError=%v, want QueueOverflowError", observed)
	}
	sts.Close() // Must also be safe before Start.
}

func TestRefreshSnapshotSkippedWhenDisposed(t *testing.T) {
	called := false
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			called = true
			return "snap", nil
		},
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		ReadPublication:       func(p int) []int { return []int{p} },
		Decode:                func([]byte) (int, error) { return 0, nil },
	})
	sts.mu.Lock()
	sts.disposed = true
	sts.mu.Unlock()

	if err := sts.RefreshSnapshot(context.Background()); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}
	if called {
		t.Fatal("did not expect fetch when disposed")
	}
}

func TestSnapshotCallbacksPanicFailClosedWithoutLockingTheCoordinator(t *testing.T) {
	var observed error
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot:         func(context.Context) (string, error) { return "snap", nil },
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) { panic("consumer bug") },
		ReadPublication:       func(p int) []int { return []int{p} },
		Decode:                func([]byte) (int, error) { return 0, nil },
		OnError:               func(err error) { observed = err },
	})
	if err := sts.RefreshSnapshot(context.Background()); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}

	sts.handlePublication(1)
	if observed == nil {
		t.Fatal("callback panic was not surfaced")
	}
	if sts.IsReady() {
		t.Fatal("callback panic must fail closed")
	}
	if sts.Err() == nil {
		t.Fatal("callback panic must remain observable through Err")
	}
	// These calls prove the mutex was released before invoking consumer code.
	sts.Close()
}

// A publication that arrives while the snapshot is being applied goes to
// ApplyDrainedPublications, before the stream is ready; one arriving after the
// refresh goes to ApplyLivePublications.
func TestPublicationsArrivingDuringApplyGoToApplyDrained(t *testing.T) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var mu sync.Mutex
	var drained, live []int
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot:   func(context.Context) (string, error) { return "snap", nil },
		ReadPublication: func(p int) []int { return []int{p} },
		ApplySnapshot: func(string, []int) {
			close(applyStarted)
			<-releaseApply
		},
		ApplyDrainedPublications: func(items []int) {
			mu.Lock()
			drained = append(drained, items...)
			mu.Unlock()
		},
		ApplyLivePublications: func(items []int) {
			mu.Lock()
			live = append(live, items...)
			mu.Unlock()
		},
		Decode: func([]byte) (int, error) { return 0, nil },
	})
	done := make(chan error, 1)
	go func() { done <- sts.RefreshSnapshot(context.Background()) }()
	<-applyStarted
	sts.handlePublication(7)
	close(releaseApply)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	sts.handlePublication(8)
	mu.Lock()
	defer mu.Unlock()
	if len(drained) != 1 || drained[0] != 7 || len(live) != 1 || live[0] != 8 {
		t.Fatalf("drained=%v live=%v, want drained [7] live [8]", drained, live)
	}
}

// A publication the coordinator dispatched live stays live even when a
// refresh starts, and marks the stream not ready, before its callback runs:
// the path is decided under the coordinator's lock, not when the callback
// reads the stream's state.
func TestALiveDispatchedPublicationStaysLiveWhenARefreshStartsFirst(t *testing.T) {
	fetches := 0
	fetchStarted := make(chan struct{})
	failFetch := make(chan struct{})
	liveEntered := make(chan struct{})
	releaseLive := make(chan struct{})
	var mu sync.Mutex
	var drained, live []int
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			mu.Lock()
			fetches++
			n := fetches
			mu.Unlock()
			if n == 1 {
				return "snap", nil
			}
			close(fetchStarted)
			<-failFetch
			return "", errors.New("snapshot unavailable")
		},
		ReadPublication: func(p int) []int { return []int{p} },
		ApplySnapshot:   func(string, []int) {},
		ApplyDrainedPublications: func(items []int) {
			mu.Lock()
			drained = append(drained, items...)
			mu.Unlock()
		},
		ApplyLivePublications: func(items []int) {
			close(liveEntered)
			<-releaseLive
			mu.Lock()
			live = append(live, items...)
			mu.Unlock()
		},
		Decode: func([]byte) (int, error) { return 0, nil },
	})
	if err := sts.RefreshSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	dispatched := make(chan struct{})
	go func() {
		sts.handlePublication(8)
		close(dispatched)
	}()
	<-liveEntered
	refreshed := make(chan error, 1)
	go func() { refreshed <- sts.RefreshSnapshot(context.Background()) }()
	<-fetchStarted
	if sts.IsReady() {
		t.Fatal("the refresh did not mark the stream not ready")
	}
	close(releaseLive)
	<-dispatched
	close(failFetch)
	if err := <-refreshed; err == nil {
		t.Fatal("the second refresh should fail")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(live) != 1 || live[0] != 8 || len(drained) != 0 {
		t.Fatalf("live=%v drained=%v, want the live-dispatched publication live", live, drained)
	}
}
