package marketstream

import (
	"context"
	"testing"
	"time"
)

// The moment the transport is lost, every book, quote and trades stream it
// fed is stale: no consumer sees a live-looking book from a dead connection.
func TestTransportLossMakesEveryStreamStaleBeforeDisconnected(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	topics := []Topic{{7, StreamDepth}, {7, StreamDepth20}, {7, StreamBBO}, {7, StreamTrades}}
	results := subscribeAsync(c, topics...)
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK),
		depthSnapshotFrame(7, StreamDepth, 1, 10, []levelMsg{lv(100, 1)}, nil),
		depthSnapshotFrame(7, StreamDepth20, 1, 10, []levelMsg{lv(100, 1)}, nil),
		bboFrame(7, 1, 10, 100, 1, 101, 1),
		tradesThroughFrame(7, 1, 100))
	awaitResults(t, results)
	expect[*TradesThrough](t, c)
	for _, tp := range topics {
		if !c.Live(tp) {
			t.Fatalf("%s not live on a healthy connection", tp)
		}
	}
	if book, _ := c.OrderBook(7, StreamDepth, 0); book.Stale {
		t.Fatal("healthy book stale")
	}

	sc.close()
	expect[*Disconnected](t, c)
	// Checked as the Disconnected event is observed, before any reconnect.
	for _, s := range []Stream{StreamDepth, StreamDepth20} {
		if book, ok := c.OrderBook(7, s, 0); !ok || !book.Stale {
			t.Fatalf("%s book %+v not stale after the transport closed", s, book)
		}
	}
	if q, ok := c.BestBidOffer(7); !ok || !q.Stale {
		t.Fatalf("quote %+v not stale after the transport closed", q)
	}
	for _, tp := range topics {
		if c.Live(tp) {
			t.Fatalf("%s live after the transport closed", tp)
		}
	}

	// A new session: stale until its snapshot and quote arrive, and, for a
	// pair reported stale, until LIVE after them.
	sc2 := fs.accept(t)
	resub := sc2.subscribe(t)
	sc2.send(t, respond(resub, SubscribeOK), marketStatusFrame(7, FeedStale), depthSnapshotFrame(7, StreamDepth, 1, 20, []levelMsg{lv(100, 2)}, nil))
	expect[*DepthSnapshot](t, c)
	if book, _ := c.OrderBook(7, StreamDepth, 0); !book.Stale {
		t.Fatal("book of a pair reported stale is live before LIVE")
	}
	sc2.send(t, marketStatusFrame(7, FeedLive), bboFrame(7, 1, 10, 100, 1, 101, 1))
	expect[*BestBidOffer](t, c) // the same state as before the loss is delivered again
	if book, _ := c.OrderBook(7, StreamDepth, 0); book.Stale || !c.Live(Topic{7, StreamDepth}) {
		t.Fatalf("book %+v stale after a fresh snapshot and LIVE", book)
	}
	if q, _ := c.BestBidOffer(7); q.Stale {
		t.Fatal("quote stale after a fresh quote")
	}
}

// An Unsubscribe that was sent is committed: cancelling the wait does not
// stop a late OK from ending the stream.
func TestUnsubscribeCancelledAfterSendingStillEndsTheStream(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamBBO})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), bboFrame(7, 1, 5, 100, 1, 101, 1))
	awaitResults(t, results)
	expect[*BestBidOffer](t, c)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Unsubscribe(ctx, Topic{7, StreamBBO})
		done <- err
	}()
	m := sc.subscribe(t)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled Unsubscribe returned no error")
	}
	sc.send(t, respond(m, SubscribeOK))
	resp := expect[*SubscribeResponse](t, c)
	if resp.RequestID != m.requestID {
		t.Fatalf("response %+v", resp)
	}
	if c.Live(Topic{7, StreamBBO}) {
		t.Fatal("stream still held after its Unsubscribe was answered")
	}
	if _, ok := c.BestBidOffer(7); ok {
		t.Fatal("quote kept after its Unsubscribe was answered")
	}
}

// An Unsubscribe whose connection drops before the answer counts as done.
func TestUnsubscribeUnansweredBeforeADropCountsAsDone(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamBBO}, Topic{7, StreamTrades})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), bboFrame(7, 1, 5, 100, 1, 101, 1))
	awaitResults(t, results)
	expect[*BestBidOffer](t, c)

	go func() { _, _ = c.Unsubscribe(context.Background(), Topic{7, StreamBBO}) }()
	sc.subscribe(t)
	sc.close()
	expect[*Disconnected](t, c)
	if _, ok := c.BestBidOffer(7); ok {
		t.Fatal("quote of an unsubscribed stream kept after the connection dropped")
	}
	resub := fs.accept(t).subscribe(t)
	if len(resub.topics) != 1 || resub.topics[0] != (Topic{7, StreamTrades}) {
		t.Fatalf("resubscribed %v", resub.topics)
	}
}

// A Subscribe cancelled after sending is committed too: its late OK
// activates the stream, which the next reconnect resubscribes.
func TestSubscribeCancelledAfterSendingStillTakesEffect(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Subscribe(ctx, Topic{7, StreamBBO})
		done <- err
	}()
	m := sc.subscribe(t)
	cancel()
	<-done
	sc.send(t, respond(m, SubscribeOK), bboFrame(7, 1, 5, 100, 1, 101, 1))
	if q := expect[*BestBidOffer](t, c); q.UpdateID != 5 {
		t.Fatalf("quote %+v", q)
	}
	if !c.Live(Topic{7, StreamBBO}) {
		t.Fatal("stream not held after its Subscribe was answered")
	}
	sc.close()
	if resub := fs.accept(t).subscribe(t); len(resub.topics) != 1 {
		t.Fatalf("resubscribed %v", resub.topics)
	}
}

// A response to no request of this connection changes nothing.
func TestAResponseToNoRequestChangesNothing(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	sc.send(t, subscribeResponseFrame(4242, resultMsg{7, uint8(StreamBBO), uint8(SubscribeOK)}), bboFrame(7, 1, 5, 100, 1, 101, 1))
	expect[*SubscribeResponse](t, c)
	time.Sleep(20 * time.Millisecond)
	if c.Live(Topic{7, StreamBBO}) {
		t.Fatal("an unrequested OK activated a stream")
	}
	if _, ok := c.BestBidOffer(7); ok {
		t.Fatal("a quote of an unrequested stream applied")
	}
}

// A book keeps its highest (epoch, update) for its whole life: a reconnect
// to a replica behind never takes it backwards.
func TestBooksAndQuotesNeverGoBackwardsAcrossReconnects(t *testing.T) {
	for _, s := range []Stream{StreamDepth20, StreamDepth} {
		t.Run(s.String(), func(t *testing.T) {
			fs := newFakeServer(t)
			c := dialTest(t, fs)
			sc := fs.accept(t)
			results := subscribeAsync(c, Topic{7, s}, Topic{7, StreamBBO})
			sub := sc.subscribe(t)
			sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, s, 1, 10, []levelMsg{lv(100, 1)}, nil), bboFrame(7, 1, 10, 100, 1, 101, 1))
			awaitResults(t, results)
			expect[*BestBidOffer](t, c)
			sc.close()
			expect[*Disconnected](t, c)

			// The new connection's replica is behind: its state is older.
			sc2 := fs.accept(t)
			resub := sc2.subscribe(t)
			sc2.send(t, respond(resub, SubscribeOK), depthSnapshotFrame(7, s, 1, 9, []levelMsg{lv(99, 1)}, nil), bboFrame(7, 1, 9, 99, 1, 101, 1), pongFrame(0))
			expect[*SubscribeResponse](t, c)
			time.Sleep(30 * time.Millisecond)
			book, _ := c.OrderBook(7, s, 0)
			if book.UpdateID != 10 || !book.Stale || book.Bids[0].Price.Ticks() != 100 {
				t.Fatalf("book %+v after an older snapshot; want update 10, stale", book)
			}
			if q, _ := c.BestBidOffer(7); q.UpdateID != 10 || !q.Stale {
				t.Fatalf("quote %+v after an older quote; want update 10, stale", q)
			}
			if s == StreamDepth {
				// Updates that catch the replica up to the book are skipped;
				// the first one at the book's base resumes it.
				sc2.send(t, depthUpdateFrame(7, 1, 10, 10, []levelMsg{lv(99, 0), lv(100, 1)}, nil), depthUpdateFrame(7, 1, 11, 11, []levelMsg{lv(100, 4)}, nil))
				if u := expect[*DepthUpdate](t, c); u.FirstUpdateID != 11 {
					t.Fatalf("update %+v", u)
				}
			} else {
				// An equal snapshot confirms the kept state.
				sc2.send(t, depthSnapshotFrame(7, s, 1, 10, []levelMsg{lv(100, 1)}, nil))
				expect[*DepthSnapshot](t, c)
			}
			sc2.send(t, bboFrame(7, 1, 10, 100, 1, 101, 1))
			expect[*BestBidOffer](t, c)
			book, _ = c.OrderBook(7, s, 0)
			if book.Stale || book.UpdateID < 10 {
				t.Fatalf("book %+v not current once the replica caught up", book)
			}
			if q, _ := c.BestBidOffer(7); q.Stale || q.UpdateID != 10 {
				t.Fatalf("quote %+v not confirmed by an equal quote", q)
			}

			// A higher epoch is a rebuilt book: taken whatever it holds.
			sc2.send(t, depthSnapshotFrame(7, s, 2, 3, []levelMsg{lv(50, 1)}, nil))
			if snap := expect[*DepthSnapshot](t, c); snap.BookEpoch != 2 {
				t.Fatalf("snapshot %+v", snap)
			}
			if book, _ := c.OrderBook(7, s, 0); book.BookEpoch != 2 || book.UpdateID != 3 || book.Stale {
				t.Fatalf("book %+v after a new epoch", book)
			}
		})
	}
}
