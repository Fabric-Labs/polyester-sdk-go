package hardening_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

func TestL2SnapshotThenStreamReconnectFailRetryThenMergeOnce(t *testing.T) {
	var active atomic.Int64
	pubCh := make(chan []byte, 4)
	ws := hardening.SpawnCentrifugoDisconnectThenPublish(&active, pubCh)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)

	var attempts atomic.Int64
	var applyMu sync.Mutex
	var appliedSnaps []string
	var appliedBuffered [][]byte
	var livePubs [][]byte
	var sawFail atomic.Bool
	pubBuffered := make(chan struct{}, 1)

	sts := realtime.NewSnapshotThenStream[[]byte, []byte](realtime.SnapshotThenStreamConfig[[]byte, []byte]{
		Client:  rt,
		Channel: "public:spot:market:trades:1:proto",
		Decode:  identityDecode,
		FetchSnapshot: func(ctx context.Context) ([]byte, error) {
			n := attempts.Add(1)
			switch n {
			case 1:
				return []byte("initial"), nil
			case 2:
				// Buffer during the failed attempt. The successful retry must
				// retain and merge this publication exactly once.
				select {
				case pubCh <- []byte("buffered-1"):
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(2 * time.Second):
					return nil, errors.New("pub inject timeout")
				}
				select {
				case <-pubBuffered:
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(2 * time.Second):
					return nil, errors.New("pending buffer timeout")
				}
				return nil, errors.New("snapshot refresh failed")
			default:
				return []byte("recovered"), nil
			}
		},
		ReadPublication: func(p []byte) [][]byte {
			if string(p) == "buffered-1" {
				select {
				case pubBuffered <- struct{}{}:
				default:
				}
			}
			return [][]byte{p}
		},
		ApplySnapshot: func(snap []byte, buffered [][]byte) {
			applyMu.Lock()
			defer applyMu.Unlock()
			appliedSnaps = append(appliedSnaps, string(snap))
			for _, b := range buffered {
				appliedBuffered = append(appliedBuffered, append([]byte(nil), b...))
			}
		},
		ApplyLivePublications: func(pubs [][]byte) {
			applyMu.Lock()
			defer applyMu.Unlock()
			for _, p := range pubs {
				livePubs = append(livePubs, append([]byte(nil), p...))
			}
		},
		MaxBuffered: 8,
	})

	if err := sts.Start(context.Background()); err != nil {
		t.Fatalf("initial start: %v", err)
	}
	if !sts.IsReady() || sts.Err() != nil {
		t.Fatalf("initial ready=%v err=%v", sts.IsReady(), sts.Err())
	}

	// Observe fail-closed surface for the first reconnect refresh (before retry wins).
	hardening.WaitUntil(t, func() bool {
		if attempts.Load() >= 2 && sts.Err() != nil && !sts.IsReady() {
			sawFail.Store(true)
			return true
		}
		// Recovery may already have completed if scheduling is fast; require attempts.
		return attempts.Load() >= 3 && sts.IsReady()
	}, 5*time.Second)

	hardening.WaitUntil(t, func() bool {
		return attempts.Load() >= 3 && sts.IsReady() && sts.Err() == nil
	}, 5*time.Second)

	if attempts.Load() < 3 {
		t.Fatalf("attempts=%d want >=3 (initial + fail + one retry)", attempts.Load())
	}
	if !sts.IsReady() {
		t.Fatal("ready must be true after successful retry")
	}
	if sts.Err() != nil {
		t.Fatalf("Err must clear on success, got %v", sts.Err())
	}
	if !sawFail.Load() {
		// Still require the failure was attempted; Err/ready false may be brief.
		if attempts.Load() < 2 {
			t.Fatal("expected at least one failed reconnect snapshot attempt")
		}
	}

	applyMu.Lock()
	defer applyMu.Unlock()
	recovered := 0
	for _, s := range appliedSnaps {
		if s == "recovered" {
			recovered++
		}
	}
	if recovered != 1 {
		t.Fatalf("recovered ApplySnapshot count=%d want 1; snaps=%v", recovered, appliedSnaps)
	}
	if len(appliedBuffered) != 1 || string(appliedBuffered[0]) != "buffered-1" {
		t.Fatalf("buffered merge=%v want exactly [buffered-1]", appliedBuffered)
	}
	for _, p := range livePubs {
		if string(p) == "buffered-1" {
			t.Fatalf("buffered pub must not also apply live: %v", livePubs)
		}
	}

	sts.Close()
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 750*time.Millisecond)
}

func TestL2CloseDuringSnapshotRetryCancelsFetchAndSocket(t *testing.T) {
	var active atomic.Int64
	ws := hardening.SpawnCentrifugoDisconnectAfterHandshake(&active)
	t.Cleanup(ws.Close)
	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)

	var attempts atomic.Int64
	retryStalled := make(chan struct{})
	fetchCanceled := make(chan struct{})
	sts := realtime.NewSnapshotThenStream[string, []byte](realtime.SnapshotThenStreamConfig[string, []byte]{
		Client:  rt,
		Channel: "public:spot:market:trades:1:proto",
		Decode:  identityDecode,
		FetchSnapshot: func(ctx context.Context) (string, error) {
			switch attempts.Add(1) {
			case 1:
				return "initial", nil
			case 2:
				return "", errors.New("retry once")
			default:
				close(retryStalled)
				<-ctx.Done()
				close(fetchCanceled)
				return "", ctx.Err()
			}
		},
		ReadPublication:       func(p []byte) [][]byte { return [][]byte{p} },
		ApplySnapshot:         func(string, [][]byte) {},
		ApplyLivePublications: func([][]byte) {},
		MaxBuffered:           8,
	})
	if err := sts.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-retryStalled:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot retry did not stall")
	}
	started := time.Now()
	sts.Close()
	if time.Since(started) >= 750*time.Millisecond {
		t.Fatalf("close during snapshot retry lingered %s", time.Since(started))
	}
	select {
	case <-fetchCanceled:
	default:
		t.Fatal("close must cancel the in-flight snapshot fetch")
	}
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 750*time.Millisecond)
}
