package hardening_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

func TestL2CloseAbortsSubscriptionPromptlyAgainstLocalWS(t *testing.T) {
	var active atomic.Int64
	ws := hardening.SpawnCentrifugoPublic(&active)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)
	sub, err := realtime.SubscribeProto(context.Background(), rt, "public:spot:market:trades:1:proto", identityDecode)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	hardening.WaitUntil(t, func() bool { return active.Load() >= 1 }, 2*time.Second)

	started := time.Now()
	sub.Close()
	hardening.WaitUntil(t, func() bool { return !subAlive(sub) }, 750*time.Millisecond)
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 750*time.Millisecond)
	if time.Since(started) >= 750*time.Millisecond {
		t.Fatalf("close lingered %s", time.Since(started))
	}
}

func TestL2ClientCloseClosesTrackedSubscriptions(t *testing.T) {
	var active atomic.Int64
	ws := hardening.SpawnCentrifugoPublic(&active)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)
	sub, err := realtime.SubscribeProto(context.Background(), rt, "public:spot:market:trades:1:proto", identityDecode)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	hardening.WaitUntil(t, func() bool { return active.Load() >= 1 }, 2*time.Second)

	started := time.Now()
	rt.Close()
	hardening.WaitUntil(t, func() bool { return !subAlive(sub) }, 750*time.Millisecond)
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 750*time.Millisecond)
	if time.Since(started) >= 750*time.Millisecond {
		t.Fatalf("Client.Close lingered %s", time.Since(started))
	}
}

func TestL2HundredSubCloseReturnsConnCountToBaseline(t *testing.T) {
	var active atomic.Int64
	ws := hardening.SpawnCentrifugoPublic(&active)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 10*time.Second)
	subs := make([]*realtime.Subscription[[]byte], 0, 100)
	for i := 0; i < 100; i++ {
		sub, err := realtime.SubscribeProto(context.Background(), rt, "public:spot:market:trades:1:proto", identityDecode)
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		subs = append(subs, sub)
	}
	hardening.WaitUntil(t, func() bool { return active.Load() >= 100 }, 5*time.Second)

	started := time.Now()
	for _, sub := range subs {
		sub.Close()
	}
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 750*time.Millisecond)
	if time.Since(started) >= 750*time.Millisecond {
		t.Fatalf("100-sub close soak exceeded 750ms: %s", time.Since(started))
	}
}

func TestL2CancelDuringCentrifugoWaitNoOrphan(t *testing.T) {
	var wsActive atomic.Int64
	ws := hardening.SpawnHangAfterAcceptCounted(&wsActive)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := realtime.SubscribeProto(ctx, rt, "public:spot:market:trades:1:proto", identityDecode)
		errCh <- err
	}()

	hardening.WaitUntil(t, func() bool { return wsActive.Load() >= 1 }, 2*time.Second)
	started := time.Now()
	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("cancel must error")
		}
		if !errors.Is(err, context.Canceled) &&
			!strings.Contains(errText(err), "cancel") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("cancel during Centrifugo wait did not return promptly")
	}
	if time.Since(started) > 750*time.Millisecond {
		t.Fatalf("cancel lingered %s", time.Since(started))
	}
	hardening.WaitUntil(t, func() bool { return wsActive.Load() == 0 }, 750*time.Millisecond)
}

func TestL2CloseDuringReconnectBackoffNoExtraConnect(t *testing.T) {
	var active atomic.Int64
	ws := hardening.SpawnCentrifugoDisconnectAfterHandshake(&active)
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), "", nil, 5*time.Second)
	sub, err := realtime.SubscribeProto(context.Background(), rt, "public:spot:market:trades:1:proto", identityDecode)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	hardening.WaitUntil(t, func() bool { return ws.Connects() >= 1 }, 2*time.Second)
	// After forced disconnect, client sleeps ~1s then reconnects. Close during backoff.
	hardening.WaitUntil(t, func() bool { return active.Load() == 0 }, 2*time.Second)
	connectsBefore := ws.Connects()
	sub.Close()
	// Observe through the reconnect backoff window without using sleep as a pass condition:
	// any extra connect must not appear while Close has completed.
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ws.Connects() != connectsBefore {
			t.Fatalf("close during reconnect backoff must not start an extra connect (%d -> %d)",
				connectsBefore, ws.Connects())
		}
		// Keep watching until the backoff window ends even after the sub dies.
		_ = subAlive(sub)
		_ = active.Load()
		time.Sleep(20 * time.Millisecond)
	}
	if ws.Connects() != connectsBefore {
		t.Fatalf("extra connect after close: before=%d after=%d", connectsBefore, ws.Connects())
	}
	if subAlive(sub) {
		t.Fatal("subscription should not be alive after Close")
	}
}
