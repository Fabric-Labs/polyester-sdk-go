package marketstream

import (
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
// requests in the order received and sends frames in order. A new depth
// subscription opens with a snapshot at the pair's position, a new bbo one
// with a quote.
type simServer struct {
	t     *testing.T
	held  map[Topic]bool
	in    [][]byte
	out   [][]byte
	pos   map[uint32]uint64
	quote uint64
	log   []string
}

func newSimServer(t *testing.T) *simServer {
	return &simServer{t: t, held: map[Topic]bool{}, pos: map[uint32]uint64{7: 10, 9: 10}, quote: 1}
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
		switch {
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
	s.log = append(s.log, fmt.Sprintf("server answers %d %v", m.requestID, results))
	s.out = append(s.out, subscribeResponseFrame(m.requestID, results...))
	for _, t := range opened {
		switch t.Stream {
		case StreamDepth:
			s.out = append(s.out, s.snapshot(t.SymbolID))
		case StreamBBO:
			s.out = append(s.out, s.bbo(t.SymbolID))
		}
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
	if err := c.handle(frame); err != nil {
		s.t.Fatalf("client refused a frame: %v\n%s", err, strings.Join(s.log, "\n"))
	}
	for len(c.events) > 0 {
		<-c.events
	}
}

// quiesce runs the server and client until neither has anything left to
// send; every held bbo stream then sends a quote, as it does on every
// change.
func (s *simServer) quiesce(c *Client) {
	for round := 0; round < 2; round++ {
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
		for _, symbol := range []uint32{7, 9} {
			if s.held[Topic{symbol, StreamBBO}] {
				s.out = append(s.out, s.bbo(symbol))
			}
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
			if c.active[t] || c.books[t] != nil || (t.Stream == StreamBBO && c.quotes[t.SymbolID] != nil) {
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

var simTopics = []Topic{{7, StreamDepth}, {9, StreamDepth}, {7, StreamBBO}}

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

// A Subscribe answered OK, superseded by a second Subscribe answered
// ALREADY_SUBSCRIBED: the snapshot that followed the OK arrived before the
// stream was held, so the stream recovers and goes live.
func TestADepthStreamHeldWithoutItsSnapshotRecovers(t *testing.T) {
	depth := Topic{7, StreamDepth}
	s := newSimServer(t)
	c := simClient(t, s)
	s.caller(c, depth, false)
	s.caller(c, depth, false)
	s.process()
	s.process()
	s.deliver(c) // OK, superseded
	s.deliver(c) // the snapshot, before the stream is held
	s.deliver(c) // ALREADY_SUBSCRIBED
	if len(s.in) != 1 || !decodeClient(t, s.in[0]).unsubscribe {
		t.Fatalf("a stream held without its snapshot did not recover\n%s", strings.Join(s.log, "\n"))
	}
	s.quiesce(c)
	s.settled(c, map[Topic]bool{depth: true})
}

// Caller requests, answers, depth updates, gaps and stale periods in random
// orders, seeded and reproducible: at quiescence the server holds exactly
// what the caller wants, and every wanted stream is live, depth with the
// server's book. POLYESTER_INTERLEAVE_SEED runs one seed.
func TestRandomInterleavingsSettleOnTheCallersIntent(t *testing.T) {
	seeds := make([]uint64, 0, 300)
	if env := os.Getenv("POLYESTER_INTERLEAVE_SEED"); env != "" {
		seed, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, seed)
	} else {
		for seed := uint64(1); seed <= 300; seed++ {
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
				case n < 88:
					s.update(symbol, false)
				case n < 96:
					s.update(symbol, true)
				default:
					s.stale(symbol)
				}
			}
			s.quiesce(c)
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
				if s.held[Topic{7, StreamBBO}] {
					s.out = append(s.out, s.bbo(7))
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
