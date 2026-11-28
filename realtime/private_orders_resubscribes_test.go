package realtime

import (
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

// dropOrders ends the server's TCP connection without a close frame, as a
// lost connection does, and waits for the consumer's OrderStreamDisconnected.
func dropOrders(t *testing.T, ws *fakeWS, sub *Subscription[OrderStreamEvent]) {
	t.Helper()
	_ = ws.ws.UnderlyingConn().Close()
	expectEvent[OrderStreamDisconnected](t, sub)
}

// resumeOrders accepts the reconnect, checks it resumes from cursor, and
// confirms it as server subscription sub.
func resumeOrders(t *testing.T, f *fakePrivateServer, sub, cursor string) *fakeWS {
	t.Helper()
	ws := f.accept(t)
	ws.expectAuth(t)
	request := ws.expectSubscribe(t)
	if request.GetCursor() != cursor {
		t.Fatalf("resume cursor %q, want %q", request.GetCursor(), cursor)
	}
	ws.confirm(t, sub, request)
	return ws
}

func expectNoResubscribe(t *testing.T, sub *Subscription[OrderStreamEvent], count uint64) {
	t.Helper()
	if got := sub.Resubscribes(); got != count {
		t.Fatalf("Resubscribes() = %d, want %d", got, count)
	}
	if sub.TakeResubscribed() {
		t.Fatal("TakeResubscribed() = true, want false")
	}
}

func expectOneResubscribeTaken(t *testing.T, sub *Subscription[OrderStreamEvent], count uint64) {
	t.Helper()
	if got := sub.Resubscribes(); got != count {
		t.Fatalf("Resubscribes() = %d, want %d", got, count)
	}
	if !sub.TakeResubscribed() {
		t.Fatal("TakeResubscribed() = false, want true")
	}
	if sub.TakeResubscribed() {
		t.Fatal("second TakeResubscribed() = true, want false")
	}
	if got := sub.Resubscribes(); got != count {
		t.Fatalf("Resubscribes() after take = %d, want %d", got, count)
	}
}

func TestPrivateOrdersEachResumeAfterAnAbruptDropCountsOnce(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, first := connectOrders(t, f, c)
	// The first connect is not a reconnect.
	expectNoResubscribe(t, sub, 0)
	seq := first.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	first.send(t, "s1", seq+1, readyFrame())
	expectEvent[OrderSnapshot](t, sub)
	expectEvent[OrderReady](t, sub)
	expectNoResubscribe(t, sub, 0)

	dropOrders(t, first, sub)
	// A gap that has not resumed yet is no reconnect either.
	expectNoResubscribe(t, sub, 0)
	second := resumeOrders(t, f, "s2", "c1")
	expectEvent[OrderStreamConnected](t, sub)
	expectOneResubscribeTaken(t, sub, 1)
	// The resumed subscription still reaches ready and moves its cursor.
	second.send(t, "s2", 1, readyFrame())
	expectEvent[OrderReady](t, sub)
	second.send(t, "s2", 2, unitFrame("c2", []*orderv1.Order{testOrder(2, 1)}, nil))
	expectEvent[OrderUnit](t, sub)
	expectNoResubscribe(t, sub, 1)

	dropOrders(t, second, sub)
	expectNoResubscribe(t, sub, 1)
	third := resumeOrders(t, f, "s3", "c2")
	expectEvent[OrderStreamConnected](t, sub)
	expectOneResubscribeTaken(t, sub, 2)
	third.send(t, "s3", 1, readyFrame())
	expectEvent[OrderReady](t, sub)
	third.send(t, "s3", 2, unitFrame("c3", []*orderv1.Order{testOrder(3, 1)}, nil))
	expectEvent[OrderUnit](t, sub)
	expectNoResubscribe(t, sub, 2)
}

func TestPrivateOrdersResumesNotTakenInBetweenCoalesce(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, first := connectOrders(t, f, c)
	first.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	dropOrders(t, first, sub)
	second := resumeOrders(t, f, "s2", "c1")
	expectEvent[OrderStreamConnected](t, sub)
	dropOrders(t, second, sub)
	resumeOrders(t, f, "s3", "c1")
	expectEvent[OrderStreamConnected](t, sub)
	// Two resumes, one take: the count has both, the latch one.
	expectOneResubscribeTaken(t, sub, 2)
}

func TestPrivateOrdersResubscribeOnTheSameConnectionCounts(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	// A sequence gap drops the server subscription and resumes from the cursor.
	ws.send(t, "s1", seq+2, unitFrame("c9", nil, nil))
	ws.expectUnsubscribe(t)
	request := ws.expectSubscribe(t)
	if request.GetCursor() != "c1" {
		t.Fatalf("resubscribe cursor %q, want c1", request.GetCursor())
	}
	expectEvent[OrderStreamDisconnected](t, sub)
	expectNoResubscribe(t, sub, 0)
	ws.confirm(t, "s2", request)
	expectEvent[OrderStreamConnected](t, sub)
	expectOneResubscribeTaken(t, sub, 1)
}

func TestPrivateOrdersConsumerFirstConnectedByAResumeCountsNoResubscribe(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, first := connectOrders(t, f, c)
	seq := first.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	first.send(t, "s1", seq+1, readyFrame())
	expectEvent[OrderSnapshot](t, sub)
	expectEvent[OrderReady](t, sub)
	dropOrders(t, first, sub)

	// Joins while the account is disconnected: the resume is its first connect.
	joiner := subscribeOrders(t, c, testAccount)
	second := resumeOrders(t, f, "s2", "c1")
	expectEvent[OrderStreamConnected](t, sub)
	expectEvent[OrderStreamConnected](t, joiner)
	expectEvent[OrderSnapshot](t, joiner)
	expectOneResubscribeTaken(t, sub, 1)
	expectNoResubscribe(t, joiner, 0)

	second.send(t, "s2", 1, readyFrame())
	expectEvent[OrderReady](t, sub)
	expectEvent[OrderReady](t, joiner)
	dropOrders(t, second, sub)
	expectEvent[OrderStreamDisconnected](t, joiner)
	resumeOrders(t, f, "s3", "c1")
	expectEvent[OrderStreamConnected](t, sub)
	expectEvent[OrderStreamConnected](t, joiner)
	expectOneResubscribeTaken(t, sub, 2)
	expectOneResubscribeTaken(t, joiner, 1)
}
