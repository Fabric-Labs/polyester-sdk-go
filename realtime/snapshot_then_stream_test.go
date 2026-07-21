package realtime

import (
	"context"
	"testing"
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
