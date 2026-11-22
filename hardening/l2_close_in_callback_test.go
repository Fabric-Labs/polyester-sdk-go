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

// The mock completes the handshake and drops the socket, so the subscription
// reports the drop to OnError from its run goroutine and reconnects.
func subscribeDropping(t *testing.T) (*realtime.Client, *realtime.Subscription[[]byte]) {
	t.Helper()
	srv := hardening.SpawnCentrifugoDisconnectAfterHandshake(nil)
	t.Cleanup(srv.Close)
	rt := newRT(srv.WSURL(), "", nil, 5*time.Second)
	t.Cleanup(rt.Close)
	sub, err := realtime.SubscribeProto(context.Background(), rt, "public:probe", identityDecode)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return rt, sub
}

func awaitClosed(t *testing.T, what string, ch <-chan struct{}, within time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(within):
		t.Fatalf("%s did not happen within %s", what, within)
	}
}

func awaitMessagesClosed(t *testing.T, sub *realtime.Subscription[[]byte]) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sub.Messages():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("Messages() did not close after the run goroutine exited")
		}
	}
}

func TestL2CloseInsideOnErrorReturnsAndStopsRunGoroutine(t *testing.T) {
	_, sub := subscribeDropping(t)

	var once sync.Once
	returned := make(chan struct{})
	sub.SetOnError(func(error) {
		once.Do(func() {
			sub.Close()
			close(returned)
		})
	})

	awaitClosed(t, "Close() inside OnError returning", returned, 5*time.Second)
	awaitClosed(t, "run goroutine exit (Done)", sub.Done(), 2*time.Second)
	awaitMessagesClosed(t, sub)
}

func TestL2ClientCloseInsideOnErrorReturnsAndStopsRunGoroutine(t *testing.T) {
	rt, sub := subscribeDropping(t)

	var once sync.Once
	returned := make(chan struct{})
	sub.SetOnError(func(error) {
		once.Do(func() {
			rt.Close()
			close(returned)
		})
	})

	awaitClosed(t, "Client.Close() inside OnError returning", returned, 5*time.Second)
	awaitClosed(t, "run goroutine exit (Done)", sub.Done(), 2*time.Second)
}

func TestL2CloseFromOtherGoroutineStillWaitsForRunGoroutine(t *testing.T) {
	_, sub := subscribeDropping(t)
	sub.SetOnError(func(error) {})

	returned := make(chan struct{})
	go func() {
		time.Sleep(500 * time.Millisecond)
		sub.Close()
		// Close from any other goroutine waits for the run goroutine, so Done
		// is already closed when it returns.
		if subAlive(sub) {
			t.Error("Close() from another goroutine returned before the run goroutine exited")
		}
		close(returned)
	}()

	awaitClosed(t, "Close() from another goroutine returning", returned, 5*time.Second)
	awaitMessagesClosed(t, sub)
}

// subscribePublishing subscribes to a mock that sends each payload of the
// returned channel as a publication, decoding with decode. sub is set before
// any publication is sent.
func subscribePublishing(t *testing.T, decode func(*realtime.Subscription[[]byte], []byte) ([]byte, error)) (*realtime.Subscription[[]byte], chan<- []byte) {
	t.Helper()
	pubs := make(chan []byte, 1)
	srv := hardening.SpawnCentrifugoPublish(nil, pubs)
	t.Cleanup(srv.Close)
	rt := newRT(srv.WSURL(), "", nil, 5*time.Second)
	t.Cleanup(rt.Close)
	var current atomic.Pointer[realtime.Subscription[[]byte]]
	sub, err := realtime.SubscribeProto(context.Background(), rt, "public:probe", func(b []byte) ([]byte, error) {
		return decode(current.Load(), b)
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	current.Store(sub)
	return sub, pubs
}

// The decoder runs on the run goroutine too: a Close from it returns.
func TestL2CloseInsideDecoderReturnsAndStopsRunGoroutine(t *testing.T) {
	var once sync.Once
	returned := make(chan struct{})
	sub, pubs := subscribePublishing(t, func(sub *realtime.Subscription[[]byte], b []byte) ([]byte, error) {
		once.Do(func() {
			sub.Close()
			close(returned)
		})
		return b, nil
	})
	pubs <- []byte("one")

	awaitClosed(t, "Close() inside the decoder returning", returned, 5*time.Second)
	awaitClosed(t, "run goroutine exit (Done)", sub.Done(), 2*time.Second)
	awaitMessagesClosed(t, sub)
}

var errProbeDecode = errors.New("probe: undecodable publication")

func failingDecode(*realtime.Subscription[[]byte], []byte) ([]byte, error) {
	return nil, errProbeDecode
}

// SetOnError calls a new callback at once with an error already recorded, on
// the caller's goroutine. When that goroutine is not running a callback of the
// subscription, a Close from that call waits for the run goroutine, here held
// inside the earlier OnError callback.
func TestL2CloseInsideSetOnErrorReplayOnOtherGoroutineWaits(t *testing.T) {
	sub, pubs := subscribePublishing(t, failingDecode)
	held := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRun := func() { releaseOnce.Do(func() { close(release) }) }
	// A failed assertion must not leave the run goroutine held for cleanup.
	t.Cleanup(releaseRun)
	var once sync.Once
	sub.SetOnError(func(error) {
		once.Do(func() {
			close(held)
			<-release
		})
	})
	pubs <- []byte("undecodable")
	awaitClosed(t, "the run goroutine calling OnError for the decode failure", held, 5*time.Second)

	replayed := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		sub.SetOnError(func(err error) {
			if !errors.Is(err, errProbeDecode) {
				t.Errorf("replayed error %v, want the decode failure", err)
			}
			close(replayed)
			sub.Close()
		})
		close(returned)
	}()
	awaitClosed(t, "SetOnError calling the new callback", replayed, 5*time.Second)
	select {
	case <-returned:
		t.Fatal("Close() from SetOnError's call on another goroutine returned while the run goroutine was still running")
	case <-time.After(300 * time.Millisecond):
	}
	if !subAlive(sub) {
		t.Fatal("run goroutine exited while held in OnError")
	}

	releaseRun()
	awaitClosed(t, "Close() returning once the run goroutine exits", returned, 5*time.Second)
	if subAlive(sub) {
		t.Fatal("Close() returned before the run goroutine exited")
	}
}

// SetOnError called from a callback on the run goroutine keeps that
// goroutine's mark: a Close from the callback it calls at once returns.
func TestL2CloseInsideSetOnErrorReplayOnRunGoroutineReturns(t *testing.T) {
	sub, pubs := subscribePublishing(t, failingDecode)
	var once sync.Once
	returned := make(chan struct{})
	sub.SetOnError(func(error) {
		once.Do(func() {
			sub.SetOnError(func(error) { sub.Close() })
			close(returned)
		})
	})
	pubs <- []byte("undecodable")

	awaitClosed(t, "Close() inside SetOnError's call on the run goroutine returning", returned, 5*time.Second)
	awaitClosed(t, "run goroutine exit (Done)", sub.Done(), 2*time.Second)
}
