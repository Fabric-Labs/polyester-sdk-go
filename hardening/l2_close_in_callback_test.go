package hardening_test

import (
	"context"
	"sync"
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
