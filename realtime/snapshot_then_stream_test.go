package realtime

import (
	"context"
	"errors"
	"testing"

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
