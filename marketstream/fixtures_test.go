package marketstream

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// The fixtures are synthetic frames derived from captured server traffic by
// internal/tools/marketstreamfixtures: every identifier, price, quantity and
// time is rewritten, and the frames are re-encoded with the generated codecs.
// testdata/fixtures/index.tsv lists them by group; a row with no file is a
// message not yet captured.

type fixtureFrame struct {
	file, direction, message, group, note string
	data                                  []byte
}

func loadFixtures(t *testing.T) (frames []fixtureFrame, todo []string) {
	t.Helper()
	dir := filepath.Join("testdata", "fixtures")
	raw, err := os.ReadFile(filepath.Join(dir, "index.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		cols := strings.Split(line, "\t")
		if strings.HasPrefix(line, "#") || len(cols) != 5 || cols[0] == "file" {
			continue
		}
		if cols[0] == "-" {
			todo = append(todo, cols[3]+": "+cols[4])
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, cols[0]))
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, fixtureFrame{file: cols[0], direction: cols[1], message: cols[2], group: cols[3], note: cols[4], data: data})
	}
	if len(frames) == 0 {
		t.Fatal("no fixtures")
	}
	return frames, todo
}

func fixtureGroup(t *testing.T, frames []fixtureFrame, group string) []fixtureFrame {
	t.Helper()
	var out []fixtureFrame
	for _, f := range frames {
		if f.group == group {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures of group %s", group)
	}
	return out
}

func decodeSubscribe(t *testing.T, data []byte) (uint64, []Topic) {
	t.Helper()
	var h ms.MessageHeader
	h.Wrap(data, 0, 0, uint64(len(data)))
	var m ms.Subscribe
	m.WrapForDecode(data, uint64(headerLength), uint64(h.BlockLength()), uint64(h.Version()), uint64(len(data)))
	var topics []Topic
	items := m.Items()
	for items.HasNext() {
		it := items.Next()
		topics = append(topics, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
	}
	return m.RequestId(), topics
}

// Every server fixture decodes to its message; every client fixture is what
// this client encodes for the same request; nothing in them is a captured
// identifier.
func TestFixturesDecodeAndEncode(t *testing.T) {
	frames, todo := loadFixtures(t)
	for _, item := range todo {
		t.Logf("TODO fixture %s", item)
	}
	seen := map[string]bool{}
	for _, f := range frames {
		seen[f.direction+" "+f.message] = true
		if f.direction == "client" {
			var again []byte
			switch f.message {
			case "Subscribe":
				id, topics := decodeSubscribe(t, f.data)
				for _, tp := range topics {
					if tp.SymbolID < 101 {
						t.Fatalf("%s: pair %d is not synthetic", f.file, tp.SymbolID)
					}
				}
				again = encodeSubscribe(id, topics)
			case "Ping":
				var h ms.MessageHeader
				h.Wrap(f.data, 0, 0, uint64(len(f.data)))
				var m ms.Ping
				m.WrapForDecode(f.data, uint64(headerLength), uint64(h.BlockLength()), 0, uint64(len(f.data)))
				again = encodePing(m.RequestId())
			}
			if !bytes.Equal(again, f.data) {
				t.Fatalf("%s: this client encodes %x, the fixture holds %x", f.file, again, f.data)
			}
			continue
		}
		got, err := decodeFrame(f.data)
		if err != nil {
			t.Fatalf("%s: %v", f.file, err)
		}
		if want := goldenTypes[f.message]; reflect.TypeOf(got) != want {
			t.Fatalf("%s: %s decoded to %T", f.file, f.message, got)
		}
	}
	for _, m := range []string{"client Subscribe", "client Ping", "server SubscribeResponse", "server Pong", "server Trades", "server TradesThrough", "server BestBidOffer", "server DepthSnapshot", "server DepthUpdate", "server MarketStatus"} {
		if !seen[m] {
			t.Errorf("no %s fixture", m)
		}
	}
}

// fixtureClient is a client with the fixture Subscribe sent and its response
// applied, as a connection that subscribed.
func fixtureClient(t *testing.T, frames []fixtureFrame) *Client {
	t.Helper()
	c := newClient(Config{Pairs: anyPairs{}, QueueSize: 1 << 12})
	t.Cleanup(c.cancel)
	id, topics := decodeSubscribe(t, fixtureGroup(t, frames, "client-subscribe")[0].data)
	c.recordSent(id, false, topics)
	if err := c.handle(fixtureGroup(t, frames, "subscribe-response")[0].data); err != nil {
		t.Fatal(err)
	}
	return c
}

func replay(t *testing.T, c *Client, frames []fixtureFrame) {
	t.Helper()
	for _, f := range frames {
		if err := c.handle(f.data); err != nil {
			t.Fatalf("%s: %v", f.file, err)
		}
	}
}

func events(c *Client) []Event {
	var out []Event
	for len(c.events) > 0 {
		out = append(out, <-c.events)
	}
	return out
}

func TestFixtureDepthChainFollowsExactBase(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	chain := fixtureGroup(t, frames, "depth-chain")
	replay(t, c, chain)
	last, _ := decodeFrame(chain[len(chain)-1].data)
	u := last.(*depthUpdateMsg)
	book, ok := c.OrderBook(u.symbolID, StreamDepth, 0)
	if !ok || book.Stale || book.UpdateID != u.last || !c.Live(Topic{u.symbolID, StreamDepth}) {
		t.Fatalf("book %+v after the chain ending at %d", book, u.last)
	}
	updates := 0
	for _, e := range events(c) {
		if _, ok := e.(*DepthUpdate); ok {
			updates++
		}
	}
	if updates != len(chain)-1 {
		t.Fatalf("%d DepthUpdate events for %d updates", updates, len(chain)-1)
	}
}

func TestFixtureBridgingRangeAdvancesThePosition(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	bridge := fixtureGroup(t, frames, "depth-bridge")
	replay(t, c, bridge)
	m, _ := decodeFrame(bridge[1].data)
	u := m.(*depthUpdateMsg)
	if u.last <= u.first || len(u.bids)+len(u.asks) != 0 {
		t.Fatalf("fixture %+v is not a bridging range", u)
	}
	if book, _ := c.OrderBook(u.symbolID, StreamDepth, 0); book.UpdateID != u.last || book.Stale {
		t.Fatalf("book %+v after a bridging range to %d", book, u.last)
	}
}

func TestFixtureDepth20AndQuote(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	snap := fixtureGroup(t, frames, "depth20-snapshot")
	quote := fixtureGroup(t, frames, "best-bid-offer")
	replay(t, c, append(append([]fixtureFrame{}, snap...), quote...))
	m, _ := decodeFrame(snap[0].data)
	s := m.(*depthSnapshotMsg)
	book, ok := c.OrderBook(s.symbolID, StreamDepth20, 0)
	if !ok || len(book.Bids) != len(s.bids) || len(book.Asks) != len(s.asks) || len(s.bids)+len(s.asks) == 0 || book.Stale {
		t.Fatalf("depth20 book %+v from a snapshot of %d bids and %d asks", book, len(s.bids), len(s.asks))
	}
	q, _ := decodeFrame(quote[0].data)
	if got, ok := c.BestBidOffer(q.(*bboMsg).symbolID); !ok || got.Stale {
		t.Fatalf("quote %+v", got)
	}
}

func TestFixtureTradesAfterTheirBaseline(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	replay(t, c, fixtureGroup(t, frames, "trades"))
	var trades *Trades
	for _, e := range events(c) {
		switch e := e.(type) {
		case *Trades:
			trades = e
		case *TradesGap:
			t.Fatalf("gap %+v between the baseline and its trades", e)
		}
	}
	if trades == nil || len(trades.Trades) == 0 {
		t.Fatal("no trades delivered after the baseline")
	}
	for _, tr := range trades.Trades {
		if tr.Price.Ticks() <= 0 || tr.Qty.Scaled() <= 0 {
			t.Fatalf("trade %+v", tr)
		}
	}
}

// A stale pair recovers in the server's order: its replacement snapshot, then
// LIVE.
func TestFixtureRecoveryOrder(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	snapshot := fixtureGroup(t, frames, "depth-chain")[0]
	stale, live := fixtureGroup(t, frames, "market-status-stale")[0], fixtureGroup(t, frames, "market-status-live")[0]
	replay(t, c, []fixtureFrame{snapshot, stale, snapshot, live})
	m, _ := decodeFrame(live.data)
	symbol := m.(*marketStatusMsg).symbolID
	if c.Feed(symbol) != FeedLive || !c.Live(Topic{symbol, StreamDepth}) {
		t.Fatalf("pair %d not live after its snapshot and LIVE", symbol)
	}
	// LIVE before the replacement snapshot is a gap of the depth stream: it
	// alone recovers.
	c2 := fixtureClient(t, frames)
	replay(t, c2, []fixtureFrame{snapshot, stale, live})
	if !recoveryRequested(c2, Topic{symbol, StreamDepth}) || c2.Live(Topic{symbol, StreamDepth}) {
		t.Fatal("LIVE before the replacement snapshot did not start the depth stream's recovery")
	}
}

func TestFixturePongAnswersItsPing(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	ping := fixtureGroup(t, frames, "client-ping")[0].data
	var h ms.MessageHeader
	h.Wrap(ping, 0, 0, uint64(len(ping)))
	var m ms.Ping
	m.WrapForDecode(ping, uint64(headerLength), uint64(h.BlockLength()), 0, uint64(len(ping)))
	c.ping.id = m.RequestId()
	replay(t, c, fixtureGroup(t, frames, "pong"))
	for _, e := range events(c) {
		if p, ok := e.(*Pong); ok && p.RequestID == m.RequestId() {
			return
		}
	}
	t.Fatal("the fixture pong does not answer the fixture ping")
}

// A depth snapshot holding levels, then its contiguous updates: the window
// is the snapshot's, each update applies at its base, the last one, a range
// without levels, advances the position to its end.
func TestFixtureDepthWithLevels(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	chain := fixtureGroup(t, frames, "depth-levels-chain")
	m, _ := decodeFrame(chain[0].data)
	snap := m.(*depthSnapshotMsg)
	if len(snap.bids) == 0 || len(snap.asks) == 0 {
		t.Fatalf("snapshot %+v holds no levels", snap)
	}
	replay(t, c, chain[:1])
	book, _ := c.OrderBook(snap.symbolID, StreamDepth, 0)
	if book.LevelsPerSide != int(snap.levelsPerSide) || len(book.Bids) != len(snap.bids) || len(book.Asks) != len(snap.asks) || book.Stale {
		t.Fatalf("book %+v from the snapshot", book)
	}
	replay(t, c, chain[1:])
	last, _ := decodeFrame(chain[len(chain)-1].data)
	end := last.(*depthUpdateMsg)
	if end.last <= end.first || len(end.bids)+len(end.asks) != 0 {
		t.Fatalf("the chain's last update %+v is not a bridging range", end)
	}
	book, _ = c.OrderBook(snap.symbolID, StreamDepth, 0)
	if book.UpdateID != end.last || book.Stale || len(book.Bids) > book.LevelsPerSide || len(book.Asks) > book.LevelsPerSide {
		t.Fatalf("book %+v after the chain ending at %d", book, end.last)
	}
	for i := 1; i < len(book.Bids); i++ {
		if book.Bids[i].Price.Ticks() >= book.Bids[i-1].Price.Ticks() {
			t.Fatalf("bids out of order: %+v", book.Bids)
		}
	}
	for i := 1; i < len(book.Asks); i++ {
		if book.Asks[i].Price.Ticks() <= book.Asks[i-1].Price.Ticks() {
			t.Fatalf("asks out of order: %+v", book.Asks)
		}
	}
}

// An update missing from the chain is a gap: the book is stale and not live,
// and the depth stream alone recovers by an Unsubscribe.
func TestFixtureDepthGapMarksTheBookStale(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	chain := fixtureGroup(t, frames, "depth-levels-chain")
	replay(t, c, chain[:2])
	if err := c.handle(chain[3].data); err != nil {
		t.Fatalf("a gap ended the connection: %v", err)
	}
	m, _ := decodeFrame(chain[0].data)
	topic := Topic{m.(*depthSnapshotMsg).symbolID, StreamDepth}
	if book, _ := c.OrderBook(topic.SymbolID, StreamDepth, 0); !book.Stale || c.Live(topic) {
		t.Fatalf("book %+v current after a gap", book)
	}
	if !recoveryRequested(c, topic) {
		t.Fatal("no Unsubscribe of the depth stream after its gap")
	}
	// Further updates are dropped while it recovers.
	if err := c.handle(chain[4].data); err != nil {
		t.Fatal(err)
	}
	if book, _ := c.OrderBook(topic.SymbolID, StreamDepth, 0); !book.Stale {
		t.Fatal("an update applied during the recovery")
	}
}

// recoveryRequested reports whether topic's owner is recovering it.
func recoveryRequested(c *Client, topic Topic) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := c.owners[topic]
	return o != nil && o.recovering
}

// TradesSkipped after the pair's baseline names exactly the trades the
// subscription will not receive live.
func TestFixtureTradesSkippedAfterTheBaseline(t *testing.T) {
	frames, _ := loadFixtures(t)
	c := fixtureClient(t, frames)
	group := fixtureGroup(t, frames, "trades-skipped")
	replay(t, c, group)
	m, _ := decodeFrame(group[1].data)
	skipped := m.(*tradesSkippedMsg)
	var gaps []*TradesGap
	for _, e := range events(c) {
		if g, ok := e.(*TradesGap); ok {
			gaps = append(gaps, g)
		}
	}
	want := TradesGap{SymbolID: skipped.symbolID, AfterTradeID: skipped.first - 1, ThroughTradeID: skipped.last, Cause: GapSkipped}
	if len(gaps) != 1 || *gaps[0] != want {
		t.Fatalf("gaps %+v, want only %+v", gaps, want)
	}
}
