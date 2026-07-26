package realtime

import (
	"context"
	"errors"
	"testing"
)

func TestRefreshSnapshotSurfacesError(t *testing.T) {
	want := errors.New("snapshot boom")
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			return "", want
		},
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		ReadPublication:       func(p int) []int { return []int{p} },
		Decode:                func([]byte) (int, error) { return 0, nil },
	})

	err := sts.RefreshSnapshot(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("RefreshSnapshot err=%v", err)
	}
	if sts.IsReady() {
		t.Fatal("ready should be false after failed refresh")
	}
	if !errors.Is(sts.Err(), want) {
		t.Fatalf("Err()=%v want %v", sts.Err(), want)
	}
}

func TestRefreshSnapshotClearsErrorOnSuccess(t *testing.T) {
	calls := 0
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("transient")
			}
			return "ok", nil
		},
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		ReadPublication:       func(p int) []int { return []int{p} },
		Decode:                func([]byte) (int, error) { return 0, nil },
	})

	_ = sts.RefreshSnapshot(context.Background())
	if sts.Err() == nil {
		t.Fatal("expected error after first failure")
	}
	if err := sts.RefreshSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sts.Err() != nil {
		t.Fatalf("expected cleared error, got %v", sts.Err())
	}
	if !sts.IsReady() {
		t.Fatal("expected ready after success")
	}
}

func TestRefreshSnapshotWithRetryFailClosed(t *testing.T) {
	calls := 0
	sts := NewSnapshotThenStream[string, int](SnapshotThenStreamConfig[string, int]{
		FetchSnapshot: func(context.Context) (string, error) {
			calls++
			return "", errors.New("still down")
		},
		ApplySnapshot:         func(string, []int) {},
		ApplyLivePublications: func([]int) {},
		ReadPublication:       func(p int) []int { return []int{p} },
		Decode:                func([]byte) (int, error) { return 0, nil },
	})

	err := sts.refreshSnapshotWithRetry(context.Background())
	if err == nil {
		t.Fatal("expected fail-closed error")
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2 (one retry)", calls)
	}
	if sts.Err() == nil {
		t.Fatal("expected Err() set")
	}
}
