package marketstream

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Each topic's requests go through its owner: an answer applies only to the
// request the owner sent last, and with none outstanding the owner moves the
// server toward the caller's intent. These tests drive a client against a
// simulated server that answers requests in the order received, with the
// caller's requests, the server's answers, depth updates, gaps and stale
// periods interleaved in every order.

// recordSent records request id as sent for topics, as the send path does;
// a Subscribe is the caller's, so its topics are wanted.
func (c *Client) recordSent(id uint64, unsubscribe bool, topics []Topic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent[id] = sentRequest{unsubscribe: unsubscribe, topics: append([]Topic(nil), topics...)}
	for _, t := range topics {
		c.ownerOf(t).gen = id
		if !unsubscribe && !c.wantSet[t] {
			c.wantSet[t] = true
			c.wanted = append(c.wanted, t)
		}
	}
}

// simServer is one connection's server: it holds subscriptions, answers
// requests in the order received and sends frames in order. A new
// subscription opens with its initial state and nothing more: a depth or
// depth20 snapshot at the pair's position, a quote, or the trades
// baseline. Streams are otherwise quiet unless a test sends updates.
type simServer struct {
	t     *testing.T
	held  map[Topic]bool
	in    [][]byte
	out   [][]byte
	pos   map[uint32]uint64
	quote uint64
	log   []string
	// refuse answers the next Unsubscribe of each topic with
	// LIMIT_EXCEEDED, leaving the subscription in place; refuseSub the
	// next Subscribe, leaving the topic not held.
	refuse, refuseSub map[Topic]bool
	// refused: the last request answered for the topic was a Subscribe
	// the server refused, which ends the caller's intent for it.
	refused map[Topic]bool
	// trade is each pair's last trade ID.
	trade map[uint32]uint64
	// connections counts connections after the first.
	reconnects int
	// unsubscribes records which answered requests were Unsubscribes.
	unsubscribes map[uint64]bool
	// ready: the client received state of the topic since the last answer
	// it received that ended or replaced the topic's subscription. Live may
	// only be true for a ready topic.
	ready map[Topic]bool
}

func newSimServer(t *testing.T) *simServer {
	return &simServer{
		t: t, held: map[Topic]bool{}, pos: map[uint32]uint64{7: 10, 9: 10}, quote: 1,
		refuse: map[Topic]bool{}, refuseSub: map[Topic]bool{}, refused: map[Topic]bool{},
		trade: map[uint32]uint64{7: 100, 9: 100}, unsubscribes: map[uint64]bool{}, ready: map[Topic]bool{},
	}
}

// simClient is an offline client whose requests go to s.
func simClient(t *testing.T, s *simServer) *Client {
	t.Helper()
	c := newClient(Config{Pairs: testPairs{}, QueueSize: 1 << 16})
	t.Cleanup(c.cancel)
	c.wire = func(frame []byte) error {
		m := decodeClient(t, frame)
		s.log = append(s.log, fmt.Sprintf("client sends %s %d %v", map[bool]string{false: "Subscribe", true: "Unsubscribe"}[m.unsubscribe], m.requestID, m.topics))
		s.in = append(s.in, frame)
		return nil
	}
	return c
}

func (s *simServer) snapshot(symbol uint32) []byte {
	p := s.pos[symbol]
	return depthSnapshotFrame(symbol, StreamDepth, 1, p, []levelMsg{lv(100, int64(p))}, nil)
}

func (s *simServer) bbo(symbol uint32) []byte {
	s.quote++
	return bboFrame(symbol, 1, s.quote, 100, 1, 101, 1)
}

// process answers the oldest request received.
func (s *simServer) process() {
	m := decodeClient(s.t, s.in[0])
	s.in = s.in[1:]
	var results []resultMsg
	var opened []Topic
	for _, t := range m.topics {
		r := SubscribeOK
		s.refused[t] = false
		switch {
		case m.unsubscribe && s.refuse[t]:
			delete(s.refuse, t)
			r = SubscribeLimitExceeded
		case !m.unsubscribe && s.refuseSub[t] && !s.held[t]:
			delete(s.refuseSub, t)
			s.refused[t] = true
			r = SubscribeLimitExceeded
		case m.unsubscribe && !s.held[t]:
			r = SubscribeNotSubscribed
		case m.unsubscribe:
			delete(s.held, t)
		case s.held[t]:
			r = SubscribeAlreadySubscribed
		default:
			s.held[t] = true
			opened = append(opened, t)
		}
		results = append(results, resultMsg{t.SymbolID, uint8(t.Stream), uint8(r)})
	}
	s.unsubscribes[m.requestID] = m.unsubscribe
	s.log = append(s.log, fmt.Sprintf("server answers %d %v", m.requestID, results))
	s.out = append(s.out, subscribeResponseFrame(m.requestID, results...))
	for _, t := range opened {
		switch t.Stream {
		case StreamDepth:
			s.out = append(s.out, s.snapshot(t.SymbolID))
		case StreamDepth20:
			p := s.pos[t.SymbolID]
			s.out = append(s.out, depthSnapshotFrame(t.SymbolID, StreamDepth20, 1, p, []levelMsg{lv(100, int64(p))}, nil))
		case StreamBBO:
			s.out = append(s.out, s.bbo(t.SymbolID))
		case StreamTrades:
			s.out = append(s.out, tradesThroughFrame(t.SymbolID, 1, s.trade[t.SymbolID]))
		}
	}
}

// trades sends the next trade of a held trades stream.
func (s *simServer) trades(symbol uint32) {
	if !s.held[Topic{symbol, StreamTrades}] {
		return
	}
	s.trade[symbol]++
	s.log = append(s.log, fmt.Sprintf("server trades %d: %d", symbol, s.trade[symbol]))
	s.out = append(s.out, tradesFrame(symbol, 1, s.trade[symbol], fill(s.trade[symbol])))
}

// refresh sends what a held subscription repeats on its own: the trades
// frontier, a quote, a depth20 snapshot.
func (s *simServer) refresh(symbol uint32) {
	s.log = append(s.log, fmt.Sprintf("server refreshes %d", symbol))
	if s.held[Topic{symbol, StreamTrades}] {
		s.out = append(s.out, tradesThroughFrame(symbol, 1, s.trade[symbol]))
	}
	if s.held[Topic{symbol, StreamBBO}] {
		s.out = append(s.out, s.bbo(symbol))
	}
	if s.held[Topic{symbol, StreamDepth20}] {
		p := s.pos[symbol]
		s.out = append(s.out, depthSnapshotFrame(symbol, StreamDepth20, 1, p, []levelMsg{lv(100, int64(p))}, nil))
	}
}

// reconnect replaces the connection: the server holds nothing on the new
// one and what was in flight is gone; the client marks everything stale
// and subscribes what it wants, as its session does.
func (s *simServer) reconnect(c *Client) {
	s.reconnects++
	s.log = append(s.log, "connection replaced")
	clear(s.held)
	clear(s.ready)
	s.in, s.out = nil, nil
	c.lose()
	if err := c.reconcile(func() []Topic { return append([]Topic(nil), c.wanted...) }); err != nil {
		s.t.Fatal(err)
	}
}

// update sends a depth update of a held stream; skip leaves out the update
// before it, a gap.
func (s *simServer) update(symbol uint32, skip bool) {
	if !s.held[Topic{symbol, StreamDepth}] {
		return
	}
	first := s.pos[symbol] + 1
	if skip {
		first++
	}
	s.pos[symbol] = first
	s.log = append(s.log, fmt.Sprintf("server updates %d to %d", symbol, first))
	s.out = append(s.out, depthUpdateFrame(symbol, 1, first, first, []levelMsg{lv(100, int64(first))}, nil))
}

// stale reports a pair stale, then its replacement snapshot, then live.
func (s *simServer) stale(symbol uint32) {
	s.log = append(s.log, fmt.Sprintf("server reports %d stale", symbol))
	s.out = append(s.out, marketStatusFrame(symbol, FeedStale))
	if s.held[Topic{symbol, StreamDepth}] {
		s.out = append(s.out, s.snapshot(symbol))
	}
	s.out = append(s.out, marketStatusFrame(symbol, FeedLive))
}

// deliver hands the client the oldest frame sent.
func (s *simServer) deliver(c *Client) {
	frame := s.out[0]
	s.out = s.out[1:]
	decoded, _ := decodeFrame(frame)
	switch m := decoded.(type) {
	case *subscribeResponseMsg:
		unsubscribe := s.unsubscribes[m.requestID]
		for _, r := range m.results {
			result := SubscribeResult(r.result)
			if result == SubscribeOK || unsubscribe && result == SubscribeNotSubscribed {
				s.ready[Topic{r.symbolID, Stream(r.stream)}] = false
			}
		}
	case *tradesThroughMsg:
		s.ready[Topic{m.symbolID, StreamTrades}] = true
	case *bboMsg:
		s.ready[Topic{m.symbolID, StreamBBO}] = true
	case *depthSnapshotMsg:
		s.ready[Topic{m.symbolID, Stream(m.stream)}] = true
	}
	defer func() {
		for _, t := range simTopics {
			if c.Live(t) && !s.ready[t] {
				s.t.Fatalf("%s live without state of the subscription it holds\n%s", t, strings.Join(s.log, "\n"))
			}
		}
	}()
	if err := c.handle(frame); err != nil {
		var refused *UnsubscribeError
		if !errors.As(err, &refused) {
			s.t.Fatalf("client refused a frame: %v\n%s", err, strings.Join(s.log, "\n"))
		}
		s.log = append(s.log, fmt.Sprintf("client ends the connection: %v", err))
		s.reconnect(c)
	}
	for len(c.events) > 0 {
		<-c.events
	}
}

// quiesce runs the server and client until neither has anything left to
// send. The streams are quiet: nothing re-sends a topic's initial state.
func (s *simServer) quiesce(c *Client) {
	for steps := 0; len(s.in) > 0 || len(s.out) > 0; steps++ {
		if steps > 10000 {
			s.t.Fatalf("no quiescence\n%s", strings.Join(s.log, "\n"))
		}
		if len(s.in) > 0 {
			s.process()
		} else {
			s.deliver(c)
		}
	}
}

// caller sends the caller's Subscribe or Unsubscribe, not waiting for it.
func (s *simServer) caller(c *Client, topic Topic, unsubscribe bool) {
	s.log = append(s.log, fmt.Sprintf("caller %s %s", map[bool]string{false: "subscribes", true: "unsubscribes"}[unsubscribe], topic))
	if _, _, err := c.issue([]Topic{topic}, unsubscribe); err != nil {
		s.t.Fatal(err)
	}
}

// settled checks the client and server at quiescence against the caller's
// intent: the server holds exactly what the caller wants, every request is
// answered, a wanted depth stream is live with a book at the server's
// position, a wanted bbo stream is live, and nothing else keeps state.
func (s *simServer) settled(c *Client, intent map[Topic]bool) {
	s.t.Helper()
	fail := func(format string, args ...any) {
		s.t.Helper()
		s.t.Fatalf(format+"\n%s", append(args, strings.Join(s.log, "\n"))...)
	}
	if err := c.Err(); err != nil {
		fail("client failed: %v", err)
	}
	for _, t := range simTopics {
		if s.held[t] != intent[t] {
			fail("server holds %s: %t, the caller wants it: %t", t, s.held[t], intent[t])
		}
		if c.wantSet[t] != intent[t] {
			fail("client wants %s: %t, the caller: %t", t, c.wantSet[t], intent[t])
		}
		if !intent[t] {
			if c.active[t] || c.books[t] != nil || (t.Stream == StreamBBO && c.quotes[t.SymbolID] != nil) ||
				(t.Stream == StreamTrades && c.trackers[t.SymbolID] != nil) {
				fail("state kept for %s, which the caller ended", t)
			}
			continue
		}
		if !c.Live(t) {
			fail("%s wanted but not live", t)
		}
		if t.Stream == StreamDepth {
			if b, ok := c.OrderBook(t.SymbolID, StreamDepth, 0); !ok || b.UpdateID != s.pos[t.SymbolID] {
				fail("%s book %+v, the server is at %d", t, b, s.pos[t.SymbolID])
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 0 {
		fail("unanswered requests %v", c.sent)
	}
	for t, o := range c.owners {
		if o.gen != 0 || o.recovering {
			fail("%s owner %+v at quiescence", t, *o)
		}
	}
}

var simTopics = []Topic{{7, StreamDepth}, {9, StreamDepth}, {7, StreamBBO}, {9, StreamTrades}, {9, StreamDepth20}}

// live subscribes topics and brings them live.
func (s *simServer) live(c *Client, intent map[Topic]bool, topics ...Topic) {
	for _, t := range topics {
		s.caller(c, t, false)
		intent[t] = true
	}
	s.quiesce(c)
	s.settled(c, intent)
}

// The caller ends a depth stream while its recovery is under way, before or
// after the recovery's Subscribe is sent: the server ends up not holding it,
// and the client keeps no state.
func TestTheCallerEndingARecoveringStreamEndsIt(t *testing.T) {
	depth := Topic{7, StreamDepth}
	for name, run := range map[string]func(s *simServer, c *Client){
		// The caller's Unsubscribe goes out while the recovery's Unsubscribe
		// is answered but the answer not read: the answer is superseded and
		// starts no Subscribe.
		"before the recovery subscribes": func(s *simServer, c *Client) {
			s.process()
			s.caller(c, depth, true)
			s.deliver(c)
			if len(s.in) != 1 {
				t.Fatalf("client sent %d requests after a superseded answer\n%s", len(s.in), strings.Join(s.log, "\n"))
			}
		},
		// The recovery's Subscribe is on the wire when the caller ends the
		// stream: its late OK is superseded, not applied.
		"while the recovery's Subscribe is on the wire": func(s *simServer, c *Client) {
			s.process()
			s.deliver(c)
			s.caller(c, depth, true)
			s.process()
			s.deliver(c)
			if c.books[depth] != nil || c.active[depth] {
				t.Fatalf("a superseded OK made state\n%s", strings.Join(s.log, "\n"))
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSimServer(t)
			c := simClient(t, s)
			intent := map[Topic]bool{}
			s.live(c, intent, depth)
			s.update(7, true)
			s.deliver(c)
			if len(s.in) != 1 || !decodeClient(t, s.in[0]).unsubscribe {
				t.Fatalf("no recovery Unsubscribe after the gap\n%s", strings.Join(s.log, "\n"))
			}
			run(s, c)
			delete(intent, depth)
			s.quiesce(c)
			s.settled(c, intent)
		})
	}
}

// The caller ends a recovering depth stream and subscribes it again while
// the recovery's requests are on the wire: the stream ends up held, live,
// with the new subscription's book; no cleanup Unsubscribe of the
// recovery's ends it.
func TestTheCallerResubscribingARecoveringStreamHoldsIt(t *testing.T) {
	depth := Topic{7, StreamDepth}
	for name, run := range map[string]func(s *simServer, c *Client){
		"after the recovery's Subscribe is answered": func(s *simServer, c *Client) {
			s.process() // the recovery's Subscribe: OK and a snapshot
			s.deliver(c)
			s.caller(c, depth, false)
		},
		"before the recovery's Subscribe is answered": func(s *simServer, c *Client) {
			s.caller(c, depth, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSimServer(t)
			c := simClient(t, s)
			intent := map[Topic]bool{}
			s.live(c, intent, depth)
			s.update(7, true)
			s.deliver(c)
			s.process() // the recovery's Unsubscribe
			s.deliver(c)
			if len(s.in) != 1 || decodeClient(t, s.in[0]).unsubscribe {
				t.Fatalf("no recovery Subscribe\n%s", strings.Join(s.log, "\n"))
			}
			s.caller(c, depth, true)
			run(s, c)
			s.update(7, false)
			s.quiesce(c)
			s.settled(c, intent)
		})
	}
}

// Two Subscribes of a quiet stream: the first answered OK, superseded by
// the second, answered ALREADY_SUBSCRIBED. The initial state that followed
// the OK (the trades baseline, a quote, a depth or depth20 snapshot) arrived
// before the stream was held and is never sent again, so the stream is
// ended and subscribed again, and goes live.
func TestAStreamHeldWithoutItsInitialStateIsSubscribedAgain(t *testing.T) {
	for _, topic := range []Topic{{7, StreamDepth}, {9, StreamDepth20}, {7, StreamBBO}, {9, StreamTrades}} {
		t.Run(topic.Stream.String(), func(t *testing.T) {
			s := newSimServer(t)
			c := simClient(t, s)
			s.caller(c, topic, false)
			s.caller(c, topic, false)
			s.process()
			s.process()
			s.deliver(c) // OK, superseded
			s.deliver(c) // the initial state, before the stream is held
			s.deliver(c) // ALREADY_SUBSCRIBED
			if len(s.in) != 1 || !decodeClient(t, s.in[0]).unsubscribe {
				t.Fatalf("a stream held without its initial state was not subscribed again\n%s", strings.Join(s.log, "\n"))
			}
			if c.Live(topic) {
				t.Fatal("live without its initial state")
			}
			if topic.Stream == StreamTrades {
				// A trade of the held subscription, whose baseline was
				// missed, is no protocol error.
				s.trades(topic.SymbolID)
				s.out = append(s.out[len(s.out)-1:], s.out[:len(s.out)-1]...)
				s.deliver(c)
			}
			s.process() // the Unsubscribe
			s.deliver(c)
			s.process() // the Subscribe: OK, then the initial state
			s.deliver(c)
			if c.Live(topic) {
				t.Fatal("live before the new subscription's initial state")
			}
			s.deliver(c)
			if !c.Live(topic) {
				t.Fatalf("not live after the new subscription's initial state\n%s", strings.Join(s.log, "\n"))
			}
			s.quiesce(c)
			s.settled(c, map[Topic]bool{topic: true})
		})
	}
}

// The old subscription's state arrives while the stream, held without its
// initial state, is being subscribed again: a quote, the periodic trades
// frontier, a depth20 snapshot. The replacement subscription is not live on
// it: only its own initial state makes it live, and the trades accounting
// carries on across it.
func TestAReplacementSubscriptionIsNotLiveOnItsPredecessorsState(t *testing.T) {
	for _, topic := range []Topic{{7, StreamBBO}, {9, StreamTrades}, {9, StreamDepth20}} {
		t.Run(topic.Stream.String(), func(t *testing.T) {
			s := newSimServer(t)
			c := simClient(t, s)
			s.caller(c, topic, false)
			s.caller(c, topic, false)
			s.process()
			s.process()
			s.deliver(c) // OK, superseded
			s.deliver(c) // the initial state, before the stream is held
			s.deliver(c) // ALREADY_SUBSCRIBED: the stream is subscribed again
			// The old subscription carries on until the Unsubscribe.
			switch topic.Stream {
			case StreamBBO:
				s.out = append(s.out, s.bbo(topic.SymbolID))
			case StreamTrades:
				s.out = append(s.out, tradesThroughFrame(topic.SymbolID, 1, s.trade[topic.SymbolID]))
			case StreamDepth20:
				p := s.pos[topic.SymbolID]
				s.out = append(s.out, depthSnapshotFrame(topic.SymbolID, StreamDepth20, 1, p, []levelMsg{lv(100, int64(p))}, nil))
			}
			s.deliver(c)
			if !c.Live(topic) {
				t.Fatalf("not live on the held subscription's state\n%s", strings.Join(s.log, "\n"))
			}
			s.process() // the Unsubscribe
			s.deliver(c)
			if c.Live(topic) {
				t.Fatal("live once its subscription ended")
			}
			s.process() // the Subscribe: OK, then the initial state
			s.deliver(c)
			if c.Live(topic) {
				t.Fatalf("live on the previous subscription's state\n%s", strings.Join(s.log, "\n"))
			}
			if topic.Stream == StreamBBO {
				if q, _ := c.BestBidOffer(topic.SymbolID); !q.Stale {
					t.Fatal("the previous subscription's quote not stale")
				}
			}
			s.deliver(c)
			if !c.Live(topic) {
				t.Fatalf("not live on its own initial state\n%s", strings.Join(s.log, "\n"))
			}
			if topic.Stream == StreamTrades {
				// A trade after the new baseline is delivered, with no gap.
				s.trades(topic.SymbolID)
				frame := s.out[len(s.out)-1]
				s.out = s.out[:len(s.out)-1]
				if err := c.handle(frame); err != nil {
					t.Fatal(err)
				}
				delivered := false
				for len(c.events) > 0 {
					switch e := (<-c.events).(type) {
					case *TradesGap:
						t.Fatalf("gap %+v across the resubscription", e)
					case *Trades:
						delivered = e.Trades[0].TradeID == s.trade[topic.SymbolID]
					}
				}
				if !delivered {
					t.Fatal("the trade after the new baseline not delivered")
				}
			}
			s.quiesce(c)
			s.settled(c, map[Topic]bool{topic: true})
		})
	}
}

// A Subscribe answered OK but superseded by an Unsubscribe, which the
// server refuses: the server may still hold the stream, so the connection
// ends, as for a refused Unsubscribe of a held stream.
func TestARefusedUnsubscribeAfterASupersededSubscribeReconnects(t *testing.T) {
	topic := Topic{9, StreamTrades}
	s := newSimServer(t)
	c := simClient(t, s)
	s.caller(c, topic, false)
	s.refuse[topic] = true
	s.caller(c, topic, true)
	s.process()
	s.process()
	s.deliver(c) // OK, superseded
	s.deliver(c) // the baseline of a stream no longer wanted
	frame := s.out[0]
	var refused *UnsubscribeError
	if err := c.handle(frame); !errors.As(err, &refused) || refused.Topic != topic || refused.Result != SubscribeLimitExceeded {
		t.Fatalf("refused Unsubscribe applied as %v\n%s", err, strings.Join(s.log, "\n"))
	}
}

// An Unsubscribe of a topic never subscribed that the server answers
// otherwise than NOT_SUBSCRIBED leaves nothing unknown: the server never
// held it.
func TestARefusedUnsubscribeOfANeverSubscribedTopicChangesNothing(t *testing.T) {
	topic := Topic{9, StreamTrades}
	s := newSimServer(t)
	c := simClient(t, s)
	s.refuse[topic] = true
	s.caller(c, topic, true)
	s.quiesce(c)
	s.settled(c, map[Topic]bool{})
}

// Caller requests, answers, depth updates, gaps and stale periods in random
// orders, seeded and reproducible: at quiescence the server holds exactly
// what the caller wants, and every wanted stream is live, depth with the
// server's book. POLYESTER_INTERLEAVE_SEED runs one seed,
// POLYESTER_INTERLEAVE_SEEDS the first n.
func TestRandomInterleavingsSettleOnTheCallersIntent(t *testing.T) {
	seeds := make([]uint64, 0, 300)
	if env := os.Getenv("POLYESTER_INTERLEAVE_SEED"); env != "" {
		seed, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, seed)
	} else {
		n := uint64(300)
		if env := os.Getenv("POLYESTER_INTERLEAVE_SEEDS"); env != "" {
			var err error
			if n, err = strconv.ParseUint(env, 10, 64); err != nil {
				t.Fatal(err)
			}
		}
		for seed := uint64(1); seed <= n; seed++ {
			seeds = append(seeds, seed)
		}
	}
	for _, seed := range seeds {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, 0x6d61726b6574))
			s := newSimServer(t)
			c := simClient(t, s)
			intent := map[Topic]bool{}
			for step := 0; step < 200; step++ {
				topic := simTopics[rng.IntN(len(simTopics))]
				symbol := []uint32{7, 9}[rng.IntN(2)]
				switch n := rng.IntN(100); {
				case n < 12:
					s.caller(c, topic, false)
					intent[topic] = true
				case n < 20:
					s.caller(c, topic, true)
					delete(intent, topic)
				case n < 45:
					if len(s.in) > 0 {
						s.process()
					}
				case n < 75:
					if len(s.out) > 0 {
						s.deliver(c)
					}
				case n < 82:
					s.update(symbol, false)
				case n < 88:
					s.update(symbol, true)
				case n < 91:
					s.trades(symbol)
				case n < 93:
					s.refresh(symbol)
				case n < 95:
					s.log = append(s.log, fmt.Sprintf("server will refuse the next Unsubscribe of %s", topic))
					s.refuse[topic] = true
				case n < 97:
					s.log = append(s.log, fmt.Sprintf("server will refuse the next Subscribe of %s", topic))
					s.refuseSub[topic] = true
				default:
					s.stale(symbol)
				}
			}
			// Refusals not yet used stay out of the settling.
			clear(s.refuse)
			clear(s.refuseSub)
			s.quiesce(c)
			// A Subscribe the server refused last ends the caller's intent.
			for t, refused := range s.refused {
				if refused {
					delete(intent, t)
				}
			}
			s.settled(c, intent)
			if !slices.ContainsFunc(s.log, func(l string) bool { return strings.HasPrefix(l, "server answers") }) {
				t.Fatal("the run exchanged no requests")
			}
		})
	}
}

// Concurrent callers, one per topic, send requests back to back while the
// client's reader applies answers, depth updates, gaps and stale periods from
// a server answering in order. After each round, once the callers stop, the
// server holds exactly what each caller last asked for and every wanted
// stream is live; no connection is lost.
func TestConcurrentCallersAndGapsSettleOnTheCallersIntent(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs, func(cfg *Config) { cfg.QueueSize = 1 << 16 })
	sc := fs.accept(t)
	var mu sync.Mutex
	var gaps atomic.Bool
	s := newSimServer(t)
	stop, serverDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(serverDone)
		rng := rand.New(rand.NewPCG(1, 2))
		tick := time.NewTicker(200 * time.Microsecond)
		defer tick.Stop()
		for {
			mu.Lock()
			select {
			case m := <-sc.in:
				if !m.ping {
					s.in = append(s.in, encodeRequest(m))
					s.process()
				}
			case <-tick.C:
				for _, symbol := range []uint32{7, 9} {
					switch n := rng.IntN(10); {
					case !gaps.Load():
					case n < 6:
						s.update(symbol, false)
					case n < 9:
						s.update(symbol, true)
					default:
						s.stale(symbol)
					}
				}
			case <-stop:
				mu.Unlock()
				return
			}
			out := s.out
			s.out = nil
			mu.Unlock()
			sc.send(t, out...)
		}
	}()
	disconnects := make(chan error, 1)
	go func() {
		for e := range c.Events() {
			if d, ok := e.(*Disconnected); ok {
				select {
				case disconnects <- d.Err:
				default:
				}
			}
		}
	}()

	intent := map[Topic]bool{}
	for round := range 25 {
		gaps.Store(true)
		var wg sync.WaitGroup
		var intentMu sync.Mutex
		for i, topic := range simTopics {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewPCG(uint64(round), uint64(i)))
				want := false
				for range 12 {
					want = rng.IntN(3) > 0
					if _, _, err := c.issue([]Topic{topic}, !want); err != nil {
						t.Error(err)
						return
					}
					time.Sleep(time.Duration(rng.IntN(400)) * time.Microsecond)
				}
				intentMu.Lock()
				intent[topic] = want
				intentMu.Unlock()
			}()
		}
		wg.Wait()
		gaps.Store(false)
		deadline := time.Now().Add(10 * time.Second)
		for {
			why := ""
			mu.Lock()
			for _, topic := range simTopics {
				switch {
				case s.held[topic] != intent[topic]:
					why = fmt.Sprintf("server holds %s: %t, wanted: %t", topic, s.held[topic], intent[topic])
				case intent[topic] && !c.Live(topic):
					why = fmt.Sprintf("%s not live", topic)
				case intent[topic] && topic.Stream == StreamDepth:
					if b, _ := c.OrderBook(topic.SymbolID, StreamDepth, 0); b.UpdateID != s.pos[topic.SymbolID] {
						why = fmt.Sprintf("%s book at %d, server at %d", topic, b.UpdateID, s.pos[topic.SymbolID])
					}
				case !intent[topic] && topic.Stream == StreamDepth:
					if _, ok := c.OrderBook(topic.SymbolID, StreamDepth, 0); ok {
						why = fmt.Sprintf("book kept for %s", topic)
					}
				}
			}
			mu.Unlock()
			c.mu.Lock()
			if len(c.sent) != 0 && why == "" {
				why = fmt.Sprintf("%d requests unanswered", len(c.sent))
			}
			c.mu.Unlock()
			if why == "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("round %d not settled: %s", round, why)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	close(stop)
	<-serverDone
	select {
	case err := <-disconnects:
		t.Fatalf("disconnected by %v", err)
	default:
	}
	if n := fs.connections(); n != 1 {
		t.Fatalf("%d connections", n)
	}
}

// encodeRequest is the frame of a decoded client request.
func encodeRequest(m clientMsg) []byte {
	if m.unsubscribe {
		return encodeUnsubscribe(m.requestID, m.topics)
	}
	return encodeSubscribe(m.requestID, m.topics)
}
