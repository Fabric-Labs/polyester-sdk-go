package marketstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// testPairs knows pair 7, BTC-USDC, with base quantity scale 3.
type testPairs struct{}

func (testPairs) BaseQuantityScaleForSymbolID(id uint32) (int, bool) { return 3, id == 7 || id == 9 }
func (testPairs) SymbolForSymbolID(id uint32) *string {
	if id != 7 {
		return nil
	}
	s := "BTC-USDC"
	return &s
}

// clientMsg is a decoded client message.
type clientMsg struct {
	ping        bool
	unsubscribe bool
	requestID   uint64
	topics      []Topic
}

type serverConn struct {
	ws  *websocket.Conn
	in  chan clientMsg
	mu  sync.Mutex
	end chan struct{}
}

func (s *serverConn) send(t *testing.T, frames ...[]byte) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range frames {
		if err := s.ws.WriteMessage(websocket.BinaryMessage, f); err != nil {
			t.Logf("server write: %v", err)
			return
		}
	}
}

func (s *serverConn) next(t *testing.T) clientMsg {
	t.Helper()
	select {
	case m := <-s.in:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no client message")
		return clientMsg{}
	}
}

// subscribe reads the next Subscribe, skipping pings.
func (s *serverConn) subscribe(t *testing.T) clientMsg {
	t.Helper()
	for {
		m := s.next(t)
		if !m.ping {
			return m
		}
	}
}

func (s *serverConn) close() { _ = s.ws.Close() }

type fakeServer struct {
	srv   *httptest.Server
	conns chan *serverConn
	count int
	mu    sync.Mutex
	// refuse holds Retry-After values: each upgrade takes the next one, if
	// any, and is refused with 503 and that Retry-After.
	refuse chan string
	// upgrades counts every upgrade request, refused ones included, with
	// the time of the last.
	upgrades int
	last     time.Time
}

func newFakeServer(t *testing.T) *fakeServer {
	fs := &fakeServer{conns: make(chan *serverConn, 8), refuse: make(chan string, 8)}
	upgrader := websocket.Upgrader{}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			http.NotFound(w, r)
			return
		}
		fs.mu.Lock()
		fs.upgrades++
		fs.last = time.Now()
		fs.mu.Unlock()
		select {
		case retryAfter := <-fs.refuse:
			w.Header().Set("Retry-After", retryAfter)
			http.Error(w, "UNAVAILABLE", http.StatusServiceUnavailable)
			return
		default:
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		fs.mu.Lock()
		fs.count++
		fs.mu.Unlock()
		sc := &serverConn{ws: ws, in: make(chan clientMsg, 64), end: make(chan struct{})}
		fs.conns <- sc
		defer close(sc.end)
		for {
			_, frame, err := ws.ReadMessage()
			if err != nil {
				return
			}
			sc.in <- decodeClient(t, frame)
		}
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fakeServer) connections() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.count
}

// upgradeRequests returns how many upgrades were requested and when the last
// one was.
func (fs *fakeServer) upgradeRequests() (int, time.Time) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.upgrades, fs.last
}

func (fs *fakeServer) url() string { return "ws" + strings.TrimPrefix(fs.srv.URL, "http") + Path }

func (fs *fakeServer) accept(t *testing.T) *serverConn {
	t.Helper()
	select {
	case sc := <-fs.conns:
		return sc
	case <-time.After(5 * time.Second):
		t.Fatal("no connection")
		return nil
	}
}

func decodeClient(t *testing.T, frame []byte) clientMsg {
	var h ms.MessageHeader
	h.Wrap(frame, 0, 0, uint64(len(frame)))
	switch h.TemplateId() {
	case ms.PingSbeTemplateID:
		var p ms.Ping
		p.WrapForDecode(frame, 8, uint64(h.BlockLength()), 0, uint64(len(frame)))
		return clientMsg{ping: true, requestID: p.RequestId()}
	case ms.UnsubscribeSbeTemplateID:
		var m ms.Unsubscribe
		m.WrapForDecode(frame, 8, uint64(h.BlockLength()), 0, uint64(len(frame)))
		out := clientMsg{unsubscribe: true, requestID: m.RequestId()}
		items := m.Items()
		for items.HasNext() {
			it := items.Next()
			out.topics = append(out.topics, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
		}
		return out
	case ms.SubscribeSbeTemplateID:
		var m ms.Subscribe
		m.WrapForDecode(frame, 8, uint64(h.BlockLength()), 0, uint64(len(frame)))
		out := clientMsg{requestID: m.RequestId()}
		items := m.Items()
		for items.HasNext() {
			it := items.Next()
			out.topics = append(out.topics, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
		}
		return out
	}
	t.Errorf("unexpected client template %d", h.TemplateId())
	return clientMsg{}
}

// respond answers a Subscribe with result for every topic.
func respond(m clientMsg, result SubscribeResult) []byte {
	var rs []resultMsg
	for _, tp := range m.topics {
		rs = append(rs, resultMsg{symbolID: tp.SymbolID, stream: uint8(tp.Stream), result: uint8(result)})
	}
	return subscribeResponseFrame(m.requestID, rs...)
}

func dialTest(t *testing.T, fs *fakeServer, mutate ...func(*Config)) *Client {
	t.Helper()
	cfg := Config{URL: fs.url(), Pairs: testPairs{}, PingInterval: time.Hour, PongTimeout: time.Hour}
	for _, m := range mutate {
		m(&cfg)
	}
	c, err := Dial(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// expect reads events until one of type T arrives, failing on a timeout.
func expect[T Event](t *testing.T, c *Client) T {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-c.Events():
			if !ok {
				t.Fatalf("events closed (err %v) waiting for %T", c.Err(), *new(T))
			}
			if want, ok := e.(T); ok {
				return want
			}
		case <-deadline:
			var zero T
			t.Fatalf("no %T", zero)
			return zero
		}
	}
}

func subscribeAsync(c *Client, topics ...Topic) chan []TopicResult {
	out := make(chan []TopicResult, 1)
	go func() {
		res, _ := c.Subscribe(context.Background(), topics...)
		out <- res
	}()
	return out
}

func awaitResults(t *testing.T, ch chan []TopicResult) []TopicResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe did not return")
		return nil
	}
}

func TestClientDeliversEveryStream(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	expect[*Connected](t, c)

	topics := []Topic{{7, StreamTrades}, {7, StreamBBO}, {7, StreamDepth20}, {7, StreamDepth}, {9, StreamTrades}}
	results := subscribeAsync(c, topics...)
	sub := sc.subscribe(t)
	if len(sub.topics) != len(topics) {
		t.Fatalf("subscribed %v", sub.topics)
	}
	rs := []resultMsg{{7, 1, 1}, {7, 2, 1}, {7, 3, 1}, {7, 4, 1}, {9, 1, uint8(SubscribeUnknownSymbol)}}
	sc.send(t,
		subscribeResponseFrame(sub.requestID, rs...),
		tradesThroughFrame(7, 1, 100),
		withHeader(pongFrame(1), 16, 99, SchemaID, SchemaVersion), // unknown template: skipped
		tradesFrame(7, 1, 500, fill(101), fill(102)),
		bboFrame(7, 1, 500, 64_999_500_000_000, 2_000, noPrice, 0),
		depthSnapshotFrame(7, StreamDepth20, 1, 500, []levelMsg{lv(64_999_500_000_000, 2_000)}, nil),
		depthSnapshotFrame(7, StreamDepth, 1, 500, []levelMsg{lv(64_999_500_000_000, 2_000)}, []levelMsg{lv(65_000_500_000_000, 1)}),
		depthUpdateFrame(7, 1, 501, 503, []levelMsg{lv(64_999_000_000_000, 5)}, []levelMsg{lv(65_000_500_000_000, 0)}),
		marketStatusFrame(7, FeedStale),
	)
	got := awaitResults(t, results)
	if len(got) != 5 || got[0].Result != SubscribeOK || got[4].Result != SubscribeUnknownSymbol || got[4].Topic != (Topic{9, StreamTrades}) {
		t.Fatalf("results %+v", got)
	}

	expect[*SubscribeResponse](t, c)
	through := expect[*TradesThrough](t, c)
	if through.LastTradeID != 100 || through.BookEpoch != 1 {
		t.Fatalf("baseline %+v", through)
	}
	trades := expect[*Trades](t, c)
	if trades.Symbol != "BTC-USDC" || len(trades.Trades) != 2 || trades.Trades[0].TradeID != 101 || trades.Trades[0].TakerSide != SideBuy {
		t.Fatalf("trades %+v", trades)
	}
	if trades.Trades[0].Price.Format() != "65000" {
		t.Fatalf("price %s", trades.Trades[0].Price.Format())
	}
	if qty, err := trades.Trades[0].Qty.Format(); err != nil || qty != "1.5" {
		t.Fatalf("qty %q %v", qty, err)
	}
	bbo := expect[*BestBidOffer](t, c)
	if bbo.Ask != nil || bbo.Bid == nil || bbo.Bid.Price.Format() != "64999.5" {
		t.Fatalf("bbo %+v", bbo)
	}
	if s := expect[*DepthSnapshot](t, c); s.Stream != StreamDepth20 {
		t.Fatalf("snapshot %+v", s)
	}
	if s := expect[*DepthSnapshot](t, c); s.Stream != StreamDepth {
		t.Fatalf("snapshot %+v", s)
	}
	if u := expect[*DepthUpdate](t, c); u.FirstUpdateID != 501 || u.LastUpdateID != 503 {
		t.Fatalf("update %+v", u)
	}
	if st := expect[*MarketStatus](t, c); st.Feed != FeedStale {
		t.Fatalf("status %+v", st)
	}

	book, ok := c.OrderBook(7, StreamDepth, 0)
	if !ok || book.UpdateID != 503 || len(book.Bids) != 2 || len(book.Asks) != 0 || !book.Stale {
		t.Fatalf("book %+v", book)
	}
	if book.Bids[1].Price.Format() != "64999" {
		t.Fatalf("second bid %s", book.Bids[1].Price.Format())
	}
	if q, ok := c.BestBidOffer(7); !ok || q.UpdateID != 500 {
		t.Fatalf("quote %+v", q)
	}
	if c.Feed(7) != FeedStale {
		t.Fatal("feed not stale")
	}
}

func TestClientDropsRefusedTopicsOnReconnect(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamBBO}, Topic{9, StreamBBO})
	sub := sc.subscribe(t)
	sc.send(t, subscribeResponseFrame(sub.requestID, resultMsg{7, 2, 1}, resultMsg{9, 2, uint8(SubscribeSymbolDisabled)}))
	awaitResults(t, results)
	sc.close()
	sc2 := fs.accept(t)
	resub := sc2.subscribe(t)
	if len(resub.topics) != 1 || resub.topics[0] != (Topic{7, StreamBBO}) {
		t.Fatalf("resubscribed %v", resub.topics)
	}
}

func TestClientHeartbeat(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs, func(cfg *Config) { cfg.PingInterval, cfg.PongTimeout = 20*time.Millisecond, 100*time.Millisecond })
	sc := fs.accept(t)
	ping := sc.next(t)
	if !ping.ping {
		t.Fatalf("first message %+v, want a ping", ping)
	}
	sc.send(t, pongFrame(ping.requestID))
	if p := expect[*Pong](t, c); p.RequestID != ping.requestID || p.RoundTrip <= 0 {
		t.Fatalf("pong %+v", p)
	}
	// No further pong: the connection is declared dead and replaced.
	d := expect[*Disconnected](t, c)
	if !strings.Contains(d.Err.Error(), "pong") {
		t.Fatalf("disconnected by %v", d.Err)
	}
	fs.accept(t)
}

func TestClientFailsWhenTheConsumerFallsBehind(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs, func(cfg *Config) { cfg.QueueSize = 4 })
	sc := fs.accept(t)
	go func() { _, _ = c.Subscribe(context.Background(), Topic{7, StreamBBO}) }()
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK))
	for i := range 10 {
		sc.send(t, bboFrame(7, 1, uint64(i+1), 100, 1, 101, 1))
	}
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("client did not stop")
	}
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(c.Err(), &overflow) {
		t.Fatalf("err %v, want QueueOverflowError", c.Err())
	}
	n := 0
	for range c.Events() {
		n++
	}
	if n > 4 {
		t.Fatalf("%d events queued past the bound", n)
	}
}

func TestClientStopsOnAnotherSchema(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	sc.send(t, withHeader(pongFrame(1), ms.PongSbeBlockLength, ms.PongSbeTemplateID, 21, 0))
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("client did not stop")
	}
	var schema *SchemaError
	if !errors.As(c.Err(), &schema) || schema.SchemaID != 21 {
		t.Fatalf("err %v, want SchemaError", c.Err())
	}
	time.Sleep(50 * time.Millisecond)
	if n := fs.connections(); n != 1 {
		t.Fatalf("%d connections: the client reconnected to a server it cannot read", n)
	}
}

func TestClientReconnectsAfterAMalformedMessage(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	sc.send(t, tradesFrame(7, 1, 1, fill(1))[:20])
	var frameErr *FrameError
	if d := expect[*Disconnected](t, c); !errors.As(d.Err, &frameErr) {
		t.Fatalf("disconnected by %v", d.Err)
	}
	fs.accept(t)
}

func TestClientBackfillsTradeGaps(t *testing.T) {
	fs := newFakeServer(t)
	fastRetries(t)
	history := &fakeHistory{trades: span(1, 110), w: constW(110)}
	c := dialTest(t, fs, func(cfg *Config) { cfg.History = history })
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamTrades})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), tradesThroughFrame(7, 1, 100), tradesSkippedFrame(7, 1, 101, 104), tradesFrame(7, 1, 9, fill(105)))
	awaitResults(t, results)
	g := expect[*TradesGap](t, c)
	if g.AfterTradeID != 100 || g.ThroughTradeID != 104 || g.Cause != GapSkipped {
		t.Fatalf("gap %+v", g)
	}
	bf := expect[*TradesBackfill](t, c)
	if bf.Err != nil || bf.Gap != *g {
		t.Fatalf("backfill %+v", bf)
	}
	if ids := tradeIDs(bf.Trades); len(ids) != 4 || ids[0] != 101 || ids[3] != 104 {
		t.Fatalf("backfilled %v", ids)
	}
}

func TestSubscribeValidatesTopics(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	for _, topics := range [][]Topic{nil, {{7, Stream(9)}}, {{8, StreamTrades}}} {
		if _, err := c.Subscribe(context.Background(), topics...); err == nil {
			t.Fatalf("Subscribe(%v) accepted", topics)
		}
	}
}

func TestDialRequiresURLAndPairs(t *testing.T) {
	if _, err := Dial(context.Background(), Config{Pairs: testPairs{}}); err == nil {
		t.Fatal("no URL accepted")
	}
	if _, err := Dial(context.Background(), Config{URL: "ws://127.0.0.1:1/ws/market.sbe"}); err == nil {
		t.Fatal("no Pairs accepted")
	}
	if _, err := Dial(context.Background(), Config{URL: "ws://127.0.0.1:1/ws/market.sbe", Pairs: testPairs{}}); err == nil {
		t.Fatal("failed first connection not returned")
	}
}

func TestCloseEndsEventsAndCalls(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	fs.accept(t)
	c.Close()
	for range c.Events() {
	}
	if _, err := c.Subscribe(context.Background(), Topic{7, StreamBBO}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Subscribe after Close: %v", err)
	}
	if c.Err() != nil {
		t.Fatalf("Err after Close: %v", c.Err())
	}
}

// A Subscribe right after Dial is sent once: the first connection's
// resubscription never repeats it.
func TestSubscribeRightAfterDialIsSentOnce(t *testing.T) {
	for range 20 {
		fs := newFakeServer(t)
		c := dialTest(t, fs)
		results := subscribeAsync(c, Topic{7, StreamBBO})
		sc := fs.accept(t)
		sub := sc.subscribe(t)
		sc.send(t, respond(sub, SubscribeOK))
		if got := awaitResults(t, results); len(got) != 1 || got[0].Result != SubscribeOK {
			t.Fatalf("results %+v", got)
		}
		select {
		case m := <-sc.in:
			t.Fatalf("second client message %+v", m)
		case <-time.After(20 * time.Millisecond):
		}
		c.Close()
	}
}

func unsubscribeAsync(c *Client, symbol uint32, topics ...Topic) chan []TopicResult {
	out := make(chan []TopicResult, 1)
	go func() {
		var res []TopicResult
		if len(topics) == 0 {
			res, _ = c.UnsubscribeSymbol(context.Background(), symbol)
		} else {
			res, _ = c.Unsubscribe(context.Background(), topics...)
		}
		out <- res
	}()
	return out
}

func TestUnsubscribeEndsAStreamAndItsState(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamTrades}, Topic{7, StreamBBO})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), tradesThroughFrame(7, 1, 100), bboFrame(7, 1, 5, 100, 1, 101, 1))
	awaitResults(t, results)
	expect[*BestBidOffer](t, c)

	unsub := unsubscribeAsync(c, 0, Topic{7, StreamBBO})
	m := sc.subscribe(t)
	if !m.unsubscribe || len(m.topics) != 1 || m.topics[0] != (Topic{7, StreamBBO}) {
		t.Fatalf("unsubscribe sent %+v", m)
	}
	sc.send(t, respond(m, SubscribeOK))
	if got := awaitResults(t, unsub); len(got) != 1 || got[0].Result != SubscribeOK {
		t.Fatalf("results %+v", got)
	}
	if _, ok := c.BestBidOffer(7); ok {
		t.Fatal("quote kept after unsubscribe")
	}
	// A late quote of the ended stream is ignored.
	sc.send(t, bboFrame(7, 1, 6, 100, 1, 101, 1), tradesFrame(7, 1, 9, fill(101)))
	if tr := expect[*Trades](t, c); tr.Trades[0].TradeID != 101 {
		t.Fatalf("trades %+v", tr)
	}
	if _, ok := c.BestBidOffer(7); ok {
		t.Fatal("quote of an ended stream applied")
	}

	// A reconnect resubscribes only what is still wanted.
	sc.close()
	sc2 := fs.accept(t)
	resub := sc2.subscribe(t)
	if len(resub.topics) != 1 || resub.topics[0] != (Topic{7, StreamTrades}) {
		t.Fatalf("resubscribed %v", resub.topics)
	}
}

func TestUnsubscribeSymbolEndsEveryStreamOfThePair(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamDepth20}, Topic{7, StreamTrades}, Topic{9, StreamBBO})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, StreamDepth20, 1, 5, []levelMsg{lv(100, 1)}, nil), marketStatusFrame(7, FeedStale))
	awaitResults(t, results)
	expect[*MarketStatus](t, c)

	unsub := unsubscribeAsync(c, 7)
	m := sc.subscribe(t)
	if !m.unsubscribe || len(m.topics) != 2 || m.topics[0] != (Topic{7, StreamDepth20}) || m.topics[1] != (Topic{7, StreamTrades}) {
		t.Fatalf("unsubscribe sent %+v", m)
	}
	sc.send(t, subscribeResponseFrame(m.requestID, resultMsg{7, 3, uint8(SubscribeOK)}, resultMsg{7, 1, uint8(SubscribeNotSubscribed)}))
	got := awaitResults(t, unsub)
	if len(got) != 2 || got[1].Result != SubscribeNotSubscribed || got[1].Result.String() != "not_subscribed" {
		t.Fatalf("results %+v", got)
	}
	if _, ok := c.OrderBook(7, StreamDepth20, 0); ok {
		t.Fatal("book kept after unsubscribe")
	}
	if c.Feed(7) != FeedLive {
		t.Fatal("feed state kept for a pair no longer held")
	}
	if res, err := c.UnsubscribeSymbol(context.Background(), 7); err != nil || res != nil {
		t.Fatalf("second UnsubscribeSymbol: %v %v", res, err)
	}
}

// Trades unsubscribed and subscribed again restart from the new baseline:
// the time in between is not reported as a gap.
func TestResubscribedTradesReportNoGap(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamTrades})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), tradesThroughFrame(7, 1, 100))
	awaitResults(t, results)
	expect[*TradesThrough](t, c)

	unsub := unsubscribeAsync(c, 0, Topic{7, StreamTrades})
	m := sc.subscribe(t)
	sc.send(t, respond(m, SubscribeOK))
	awaitResults(t, unsub)

	results = subscribeAsync(c, Topic{7, StreamTrades})
	sub = sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), tradesThroughFrame(7, 1, 200), tradesFrame(7, 1, 9, fill(201)))
	awaitResults(t, results)
	for {
		select {
		case e := <-c.Events():
			if g, ok := e.(*TradesGap); ok {
				t.Fatalf("gap %+v after a deliberate resubscribe", g)
			}
			if tr, ok := e.(*Trades); ok && tr.Trades[0].TradeID == 201 {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no trades")
		}
	}
}

// A stale pair recovers with a replacement snapshot, then LIVE.
func TestDepthRecoversWithASnapshotBeforeLive(t *testing.T) {
	fs := newFakeServer(t)
	c := dialTest(t, fs)
	sc := fs.accept(t)
	results := subscribeAsync(c, Topic{7, StreamDepth})
	sub := sc.subscribe(t)
	sc.send(t, respond(sub, SubscribeOK), depthSnapshotFrame(7, StreamDepth, 1, 10, []levelMsg{lv(100, 1)}, nil), marketStatusFrame(7, FeedStale))
	awaitResults(t, results)
	expect[*MarketStatus](t, c)
	if book, _ := c.OrderBook(7, StreamDepth, 0); !book.Stale {
		t.Fatal("book of a stale pair not stale")
	}
	sc.send(t, depthSnapshotFrame(7, StreamDepth, 1, 40, []levelMsg{lv(101, 2)}, nil), marketStatusFrame(7, FeedLive), depthUpdateFrame(7, 1, 41, 41, nil, []levelMsg{lv(102, 1)}))
	expect[*DepthSnapshot](t, c)
	if st := expect[*MarketStatus](t, c); st.Feed != FeedLive {
		t.Fatalf("status %+v", st)
	}
	expect[*DepthUpdate](t, c)
	book, _ := c.OrderBook(7, StreamDepth, 0)
	if book.Stale || book.UpdateID != 41 || book.Bids[0].Qty.Scaled() != 2 {
		t.Fatalf("recovered book %+v", book)
	}
}
