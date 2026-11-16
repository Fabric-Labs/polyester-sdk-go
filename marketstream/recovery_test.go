package marketstream

import (
	"errors"
	"testing"
	"time"
)

// A depth stream recovers from a gap on its own connection: it alone goes
// stale, is unsubscribed and subscribed again, and the new subscription's
// snapshot replaces its book. These tests drive that recovery from the server
// side.

// recoveryStart subscribes depth, trades and bbo of pair 7, delivers a
// healthy state for each, then a depth update past a missing one.
func recoveryStart(t *testing.T) (*Client, *fakeServer, *serverConn) {
	t.Helper()
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamDepth}, Topic{7, StreamTrades}, Topic{7, StreamBBO})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK),
		depthSnapshotFrame(7, StreamDepth, 1, 10, []levelMsg{lv(100, 1)}, nil),
		tradesThroughFrame(7, 1, 100),
		bboFrame(7, 1, 10, 100, 1, 101, 1),
		depthUpdateFrame(7, 1, 11, 11, []levelMsg{lv(100, 2)}, nil),
		tradesFrame(7, 1, 11, fill(101)))
	awaitResults(t, results)
	expect[*Trades](t, c)
	sc.send(t, depthUpdateFrame(7, 1, 13, 13, []levelMsg{lv(100, 3)}, nil)) // 12 is missing
	return c, fs, sc
}

// nextRequest is the client's next Subscribe or Unsubscribe; it must be for
// pair 7's depth stream alone.
func nextRequest(t *testing.T, sc *serverConn, unsubscribe bool) clientMsg {
	t.Helper()
	m := sc.subscribe(t)
	if m.unsubscribe != unsubscribe || len(m.topics) != 1 || m.topics[0] != (Topic{7, StreamDepth}) {
		t.Fatalf("client sent %+v, want an %s of pair 7's depth alone", m, map[bool]string{true: "Unsubscribe", false: "Subscribe"}[unsubscribe])
	}
	return m
}

func noRequest(t *testing.T, sc *serverConn) {
	t.Helper()
	select {
	case m := <-sc.in:
		if !m.ping {
			t.Fatalf("client sent %+v", m)
		}
	case <-time.After(30 * time.Millisecond):
	}
}

func noDisconnect(t *testing.T, c *Client) {
	t.Helper()
	for {
		select {
		case e := <-c.Events():
			if d, ok := e.(*Disconnected); ok {
				t.Fatalf("disconnected by %v", d.Err)
			}
		default:
			return
		}
	}
}

// A gap on the depth stream recovers that stream alone: trades and bbo keep
// their state and stay live, updates and old snapshots that arrive while it
// recovers are dropped, a second gap starts nothing more, and the new
// snapshot makes the book current again.
func TestADepthGapRecoversOnlyThatStream(t *testing.T) {
	c, fs, sc := recoveryStart(t)
	unsub := nextRequest(t, sc, true)
	barrier(t, c, sc)
	if book, _ := c.OrderBook(7, StreamDepth, 0); !book.Stale || book.UpdateID != 11 {
		t.Fatalf("book %+v during recovery", book)
	}
	if c.Live(Topic{7, StreamDepth}) {
		t.Fatal("depth live during its recovery")
	}
	for _, other := range []Topic{{7, StreamTrades}, {7, StreamBBO}} {
		if !c.Live(other) {
			t.Fatalf("%s not live while depth recovers", other)
		}
	}
	if q, _ := c.BestBidOffer(7); q.Stale || q.UpdateID != 10 {
		t.Fatalf("quote %+v while depth recovers", q)
	}

	// While it recovers: updates, an old snapshot and a second gap.
	sc.send(t,
		depthUpdateFrame(7, 1, 14, 14, []levelMsg{lv(100, 9)}, nil),
		depthSnapshotFrame(7, StreamDepth, 1, 12, []levelMsg{lv(99, 9)}, nil),
		depthUpdateFrame(7, 1, 40, 40, nil, nil),
		tradesFrame(7, 1, 12, fill(102)))
	if tr := expect[*Trades](t, c); tr.Trades[0].TradeID != 102 {
		t.Fatalf("trades %+v while depth recovers", tr)
	}
	noRequest(t, sc)

	sc.send(t, respond(unsub, SubscribeOK), depthUpdateFrame(7, 1, 15, 15, []levelMsg{lv(100, 8)}, nil))
	sub := nextRequest(t, sc, false)
	barrier(t, c, sc)
	if book, _ := c.OrderBook(7, StreamDepth, 0); !book.Stale || book.UpdateID != 11 || book.Bids[0].Qty.Scaled() != 2 {
		t.Fatalf("book %+v changed by updates during recovery", book)
	}

	sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 30, []levelMsg{lv(100, 5)}, nil), depthUpdateFrame(7, 1, 31, 31, []levelMsg{lv(100, 6)}, nil))
	if u := expect[*DepthUpdate](t, c); u.FirstUpdateID != 31 {
		t.Fatalf("update %+v after recovery", u)
	}
	if book, _ := c.OrderBook(7, StreamDepth, 0); book.Stale || book.UpdateID != 31 || book.Bids[0].Qty.Scaled() != 6 || !c.Live(Topic{7, StreamDepth}) {
		t.Fatalf("book %+v after recovery", book)
	}
	noDisconnect(t, c)
	if n := fs.connections(); n != 1 {
		t.Fatalf("%d connections: the recovery reconnected", n)
	}
}

// NOT_SUBSCRIBED answers the recovery's Unsubscribe when the server no longer
// held the stream: the recovery goes on with the Subscribe.
func TestARecoveryUnsubscribeAnsweredNotSubscribedGoesOn(t *testing.T) {
	c, _, sc := recoveryStart(t)
	unsub := nextRequest(t, sc, true)
	sc.send(t, respond(unsub, SubscribeNotSubscribed))
	sub := nextRequest(t, sc, false)
	sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 30, []levelMsg{lv(100, 5)}, nil))
	expect[*DepthSnapshot](t, c)
	if book, _ := c.OrderBook(7, StreamDepth, 0); book.Stale || book.UpdateID != 30 {
		t.Fatalf("book %+v after recovery", book)
	}
	noDisconnect(t, c)
}

// An Unsubscribe answered otherwise leaves the stream in a state the client
// cannot know: the connection ends and every stream is resubscribed.
func TestARefusedRecoveryUnsubscribeReconnects(t *testing.T) {
	c, fs, sc := recoveryStart(t)
	unsub := nextRequest(t, sc, true)
	sc.send(t, respond(unsub, SubscribeLimitExceeded))
	var gap *BookGapError
	if d := expect[*Disconnected](t, c); !errors.As(d.Err, &gap) {
		t.Fatalf("disconnected by %v", d.Err)
	}
	if resub := fs.accept(t).subscribe(t); len(resub.topics) != 3 {
		t.Fatalf("resubscribed %v", resub.topics)
	}
}

// A lost transport still reconnects everything, a recovery in progress
// included: every wanted stream is resubscribed and rebuilt.
func TestReconnectDuringARecoveryRestoresEveryStream(t *testing.T) {
	c, fs, sc := recoveryStart(t)
	nextRequest(t, sc, true)
	sc.close()
	expect[*Disconnected](t, c)
	sc2 := fs.accept(t)
	resub := sc2.subscribe(t)
	if len(resub.topics) != 3 {
		t.Fatalf("resubscribed %v", resub.topics)
	}
	sc2.send(t, respond(resub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 50, []levelMsg{lv(100, 7)}, nil),
		tradesThroughFrame(7, 1, 101), bboFrame(7, 1, 50, 100, 1, 101, 1))
	expect[*BestBidOffer](t, c)
	for _, tp := range []Topic{{7, StreamDepth}, {7, StreamTrades}, {7, StreamBBO}} {
		if !c.Live(tp) {
			t.Fatalf("%s not live after the reconnect", tp)
		}
	}
	if book, _ := c.OrderBook(7, StreamDepth, 0); book.UpdateID != 50 {
		t.Fatalf("book %+v after the reconnect", book)
	}
	// The reconnect's snapshot ended the recovery: updates apply again.
	sc2.send(t, depthUpdateFrame(7, 1, 51, 51, nil, nil))
	if u := expect[*DepthUpdate](t, c); u.LastUpdateID != 51 {
		t.Fatalf("update %+v", u)
	}
}

// A stale pair that is reported live before its depth book's replacement
// snapshot, and an update while stale, are gaps of the depth stream alone.
func TestDepthRecoveryOutOfOrderRecoversTheStream(t *testing.T) {
	for name, frames := range map[string][][]byte{
		"live before the snapshot": {marketStatusFrame(7, FeedStale), marketStatusFrame(7, FeedLive)},
		"update while stale":       {marketStatusFrame(7, FeedStale), depthUpdateFrame(7, 1, 11, 11, nil, nil)},
	} {
		t.Run(name, func(t *testing.T) {
			fs := newFakeServer(t)
			c := dialTest(t, fs)
			sc := fs.accept(t)
			results := subscribeAsync(c, Topic{7, StreamDepth}, Topic{7, StreamBBO})
			sub := sc.subscribe(t)
			sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 10, []levelMsg{lv(100, 1)}, nil))
			awaitResults(t, results)
			sc.send(t, frames...)
			unsub := nextRequest(t, sc, true)
			sc.send(t, respond(unsub, SubscribeOK))
			resub := nextRequest(t, sc, false)
			sc.send(t, respond(resub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 20, []levelMsg{lv(100, 2)}, nil), marketStatusFrame(7, FeedLive))
			expect[*DepthSnapshot](t, c)
			barrier(t, c, sc)
			if book, _ := c.OrderBook(7, StreamDepth, 0); book.Stale || book.UpdateID != 20 {
				t.Fatalf("book %+v after recovery", book)
			}
			noDisconnect(t, c)
		})
	}
}

// An update before any snapshot of a connection's depth subscription is a
// gap: the stream recovers on that connection and is not live meanwhile.
func TestADepthUpdateBeforeASnapshotRecoversTheStream(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamDepth})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), depthUpdateFrame(7, 1, 10, 10, nil, nil))
	awaitResults(t, results)
	nextRequest(t, sc, true)
	if c.Live(Topic{7, StreamDepth}) {
		t.Fatal("depth live without a snapshot")
	}
	noDisconnect(t, c)
}

// The caller's Unsubscribe while a stream recovers ends it: the recovery
// does not subscribe it again.
func TestUnsubscribeDuringARecoveryEndsTheStream(t *testing.T) {
	c, _, sc := recoveryStart(t)
	recoveryUnsub := nextRequest(t, sc, true)
	done := make(chan []TopicResult, 1)
	go func() {
		res, _ := c.Unsubscribe(t.Context(), Topic{7, StreamDepth})
		done <- res
	}()
	userUnsub := nextRequest(t, sc, true)
	sc.send(t, respond(recoveryUnsub, SubscribeOK), respond(userUnsub, SubscribeNotSubscribed))
	if got := <-done; len(got) != 1 || got[0].Result != SubscribeNotSubscribed {
		t.Fatalf("results %+v", got)
	}
	noRequest(t, sc)
	if _, ok := c.OrderBook(7, StreamDepth, 0); ok {
		t.Fatal("book kept after the caller's Unsubscribe")
	}
}

// recoveryClient is an offline client whose pair 7 depth stream was
// subscribed and is recovering: its Unsubscribe was answered and its
// Subscribe, request 3, sent. The caller's Unsubscribe, request 2, went out
// in between.
func recoveryClient(t *testing.T) (*Client, Topic) {
	t.Helper()
	c := newClient(Config{Pairs: testPairs{}, QueueSize: 64})
	t.Cleanup(c.cancel)
	topic := Topic{7, StreamDepth}
	c.active[topic] = true
	c.books[topic] = &book{stream: StreamDepth, has: true, recovery: recoverySubscribing}
	c.sent[2] = sentRequest{unsubscribe: true, topics: []Topic{topic}}
	c.sent[3] = sentRequest{recovery: true, topics: []Topic{topic}}
	return c, topic
}

func cleanupUnsubscribe(c *Client, topic Topic) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.sent {
		if r.unsubscribe && !r.recovery && len(r.topics) == 1 && r.topics[0] == topic {
			return true
		}
	}
	return false
}

// The caller ended the stream while its recovery Subscribe was on the wire:
// the late OK recreated the server's subscription, so the client ends it
// again and keeps no state.
func TestALateRecoverySubscribeAfterTheCallerEndedTheStreamIsUndone(t *testing.T) {
	c, topic := recoveryClient(t)
	if err := c.handle(subscribeResponseFrame(2, resultMsg{7, uint8(StreamDepth), uint8(SubscribeNotSubscribed)})); err != nil {
		t.Fatal(err)
	}
	if err := c.handle(subscribeResponseFrame(3, resultMsg{7, uint8(StreamDepth), uint8(SubscribeOK)})); err != nil {
		t.Fatal(err)
	}
	if !cleanupUnsubscribe(c, topic) {
		t.Fatal("no Unsubscribe ended the subscription the late recovery Subscribe recreated")
	}
	if c.active[topic] || c.books[topic] != nil {
		t.Fatal("state kept for a stream the caller ended")
	}
}

// The caller ended the stream and subscribed it again while its recovery
// Subscribe was on the wire: the stream is held, and waits for its
// snapshot.
func TestALateRecoverySubscribeAfterTheCallerResubscribedHoldsTheStream(t *testing.T) {
	c, topic := recoveryClient(t)
	if err := c.handle(subscribeResponseFrame(2, resultMsg{7, uint8(StreamDepth), uint8(SubscribeNotSubscribed)})); err != nil {
		t.Fatal(err)
	}
	c.wantSet[topic] = true
	c.wanted = append(c.wanted, topic)
	if err := c.handle(subscribeResponseFrame(3, resultMsg{7, uint8(StreamDepth), uint8(SubscribeOK)})); err != nil {
		t.Fatal(err)
	}
	if cleanupUnsubscribe(c, topic) {
		t.Fatal("a wanted stream unsubscribed")
	}
	if !c.active[topic] || c.books[topic] == nil {
		t.Fatal("the wanted stream is not held")
	}
	if err := c.handle(depthSnapshotFrame(7, StreamDepth, 1, 40, []levelMsg{lv(100, 1)}, nil)); err != nil {
		t.Fatal(err)
	}
	if !c.Live(topic) {
		t.Fatal("the stream is not live after its snapshot")
	}
}
