package marketstream

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
)

// Pairs supplies what the client needs to know about a trading pair: its base
// quantity scale, which every quantity on the stream is scaled by, and its
// symbol. *catalogs.Manager implements it.
type Pairs interface {
	BaseQuantityScaleForSymbolID(symbolID uint32) (scale int, ok bool)
	SymbolForSymbolID(symbolID uint32) *string
}

// Config configures a Client. URL and Pairs are required.
type Config struct {
	// URL is the stream's WebSocket URL; URL(base) builds it from the API's
	// WebSocket host.
	URL string
	// Pairs supplies pair scales and symbols, usually the client's catalogs.
	Pairs Pairs
	// History, when set, recovers every TradesGap from the trade history
	// API in the background and delivers the outcome as a TradesBackfill.
	History TradeHistory
	// QueueSize bounds the Events channel. A consumer that falls behind by
	// more fails the client with a QueueOverflowError instead of losing
	// messages. Default 1000.
	QueueSize int
	// PingInterval is how often the client pings the server; a pong must
	// answer within PongTimeout or the client reconnects. Defaults 15 and 10
	// seconds.
	PingInterval time.Duration
	PongTimeout  time.Duration
	// Dialer dials the WebSocket; a default dialer with a 10 second handshake
	// timeout and the environment's proxy when nil.
	Dialer *websocket.Dialer
}

// URL is the market-data stream URL on the WebSocket host base, such as
// Environment.WebsocketURL ("wss://api.testnet.polyester.com").
func URL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", &sdkerrors.ValidationError{Msg: "market stream base URL: " + redact.URLError(err).Error()}
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return "", &sdkerrors.ValidationError{Msg: "market stream base URL must use wss or ws"}
	}
	if u.Host == "" {
		return "", &sdkerrors.ValidationError{Msg: "market stream base URL has no host"}
	}
	if u.RawQuery != "" || u.ForceQuery {
		return "", &sdkerrors.ValidationError{Msg: "market stream base URL must not contain a query string"}
	}
	if u.Fragment != "" {
		return "", &sdkerrors.ValidationError{Msg: "market stream base URL must not contain a fragment"}
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/connection/websocket")
	if !strings.HasSuffix(path, Path) {
		path += Path
	}
	u.Path = path
	return u.String(), nil
}

// ErrClosed is returned by calls on a closed Client.
var ErrClosed = &sdkerrors.RealtimeError{Msg: "market stream client is closed"}

const (
	defaultQueueSize    = 1000
	defaultPingInterval = 15 * time.Second
	defaultPongTimeout  = 10 * time.Second
	// maxMessageBytes bounds one server message: a full book of the widest
	// window the protocol carries fits.
	maxMessageBytes = 4 << 20
)

// Client is a connection to the market-data stream that reconnects and
// resubscribes on its own. Events delivers every stream's messages; the
// book and quote accessors read the latest state.
type Client struct {
	cfg    Config
	events chan Event
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	ids    atomic.Uint64
	// healthy is set once a connection delivered a message, so the next
	// reconnect starts from the shortest backoff.
	healthy atomic.Bool

	mu      sync.Mutex
	err     error
	closed  bool
	conn    *websocket.Conn
	writeMu sync.Mutex
	// wanted are the topics to hold, in the order first asked; each is
	// resubscribed on reconnect until the server refuses it.
	wanted  []Topic
	wantSet map[Topic]bool
	waiting map[uint64]chan subscribeOutcome
	// unsubscribes are the requests in flight that are Unsubscribes: their
	// responses end streams.
	unsubscribes map[uint64]bool
	ping         struct {
		id   uint64
		sent time.Time
	}
	backfills sync.WaitGroup

	// state is read by the accessors; the reader goroutine writes it.
	state    sync.RWMutex
	books    map[Topic]*book
	quotes   map[uint32]*BestBidOffer
	feeds    map[uint32]FeedState
	trackers map[uint32]*tradeTracker
	// active are the topics the current connection holds.
	active map[Topic]bool
}

type subscribeOutcome struct {
	resp *SubscribeResponse
	err  error
}

// Dial connects to the market-data stream. A failure of this first
// connection is returned; once connected, the client reconnects on its own
// until Close. Subscribe to topics with Subscribe.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, &sdkerrors.ValidationError{Msg: "market stream Config.URL is required"}
	}
	if cfg.Pairs == nil {
		return nil, &sdkerrors.ValidationError{Msg: "market stream Config.Pairs is required: quantities are scaled by each pair's base quantity scale"}
	}
	if cfg.QueueSize < 0 || cfg.PingInterval < 0 || cfg.PongTimeout < 0 {
		return nil, &sdkerrors.ValidationError{Msg: "market stream Config values must not be negative"}
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = defaultQueueSize
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = defaultPingInterval
	}
	if cfg.PongTimeout == 0 {
		cfg.PongTimeout = defaultPongTimeout
	}
	if cfg.Dialer == nil {
		cfg.Dialer = &websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	}
	c := newClient(cfg)
	conn, err := c.dial(ctx)
	if err != nil {
		c.cancel()
		return nil, err
	}
	// Subscribe may run as soon as Dial returns: the connection is the
	// client's before then.
	wanted, _ := c.attach(conn)
	go c.run(conn, wanted)
	return c, nil
}

// newClient builds a client of a validated cfg, not yet connected.
func newClient(cfg Config) *Client {
	runCtx, cancel := context.WithCancel(context.Background())
	return &Client{
		cfg:          cfg,
		events:       make(chan Event, cfg.QueueSize),
		ctx:          runCtx,
		cancel:       cancel,
		done:         make(chan struct{}),
		wantSet:      map[Topic]bool{},
		waiting:      map[uint64]chan subscribeOutcome{},
		unsubscribes: map[uint64]bool{},
		books:        map[Topic]*book{},
		quotes:       map[uint32]*BestBidOffer{},
		feeds:        map[uint32]FeedState{},
		trackers:     map[uint32]*tradeTracker{},
		active:       map[Topic]bool{},
	}
}

// Events delivers the stream's messages in order. It closes when the client
// stops: after Close, or a terminal error that Err reports.
func (c *Client) Events() <-chan Event { return c.events }

// Done is closed once the client has stopped.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err is the terminal error that stopped the client: a QueueOverflowError,
// or a SchemaError for a server this client cannot read. Nil while running
// and after Close.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close stops the client and waits until its goroutines have ended.
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	conn := c.conn
	c.mu.Unlock()
	c.cancel()
	if conn != nil {
		_ = conn.Close()
	}
	<-c.done
}

// Subscribe subscribes to topics and returns one result per topic, in order.
// Topics the server accepts are resubscribed on every reconnect; those it
// refuses are dropped. Each topic's pair must be in Config.Pairs.
func (c *Client) Subscribe(ctx context.Context, topics ...Topic) ([]TopicResult, error) {
	if len(topics) == 0 {
		return nil, &sdkerrors.ValidationError{Msg: "market stream Subscribe needs at least one topic"}
	}
	for _, t := range topics {
		if !t.Stream.known() {
			return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("market stream topic %s: unknown stream", t)}
		}
		if _, ok := c.cfg.Pairs.BaseQuantityScaleForSymbolID(t.SymbolID); !ok {
			return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("market stream topic %s: the pair's base quantity scale is unknown; load the catalogs first", t)}
		}
	}
	var results []TopicResult
	for start := 0; start < len(topics); start += maxSubscribeItems {
		chunk := topics[start:min(start+maxSubscribeItems, len(topics))]
		resp, err := c.request(ctx, chunk, false)
		if err != nil {
			return results, err
		}
		results = append(results, resp.Results...)
	}
	return results, nil
}

// request sends one Subscribe and waits for its response.
// Unsubscribe ends subscriptions and returns one result per topic, in order:
// SubscribeOK when the stream ended, SubscribeNotSubscribed when the
// connection did not hold it. The topics are not resubscribed on reconnect
// from the moment the request is sent, and their books, quotes and trade
// accounting are dropped once the server answers, so subscribing again
// starts afresh.
func (c *Client) Unsubscribe(ctx context.Context, topics ...Topic) ([]TopicResult, error) {
	if len(topics) == 0 {
		return nil, &sdkerrors.ValidationError{Msg: "market stream Unsubscribe needs at least one topic"}
	}
	for _, t := range topics {
		if !t.Stream.known() {
			return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("market stream topic %s: unknown stream", t)}
		}
	}
	var results []TopicResult
	for start := 0; start < len(topics); start += maxSubscribeItems {
		chunk := topics[start:min(start+maxSubscribeItems, len(topics))]
		resp, err := c.request(ctx, chunk, true)
		if err != nil {
			return results, err
		}
		results = append(results, resp.Results...)
	}
	return results, nil
}

// UnsubscribeSymbol ends every stream of a pair the client subscribed to, as
// Unsubscribe does; no results when it holds none.
func (c *Client) UnsubscribeSymbol(ctx context.Context, symbolID uint32) ([]TopicResult, error) {
	held := map[Topic]bool{}
	var topics []Topic
	add := func(t Topic) {
		if t.SymbolID == symbolID && !held[t] {
			held[t] = true
			topics = append(topics, t)
		}
	}
	c.mu.Lock()
	for _, t := range c.wanted {
		add(t)
	}
	c.mu.Unlock()
	c.state.RLock()
	for _, s := range []Stream{StreamTrades, StreamBBO, StreamDepth20, StreamDepth} {
		if c.active[Topic{SymbolID: symbolID, Stream: s}] {
			add(Topic{SymbolID: symbolID, Stream: s})
		}
	}
	c.state.RUnlock()
	if len(topics) == 0 {
		return nil, nil
	}
	return c.Unsubscribe(ctx, topics...)
}

// request sends one Subscribe, or Unsubscribe, and waits for its response.
func (c *Client) request(ctx context.Context, topics []Topic, unsubscribe bool) (*SubscribeResponse, error) {
	id := c.ids.Add(1)
	reply := make(chan subscribeOutcome, 1)
	c.mu.Lock()
	if c.closed || c.err != nil {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	for _, t := range topics {
		if unsubscribe {
			c.unwant(t)
		} else if !c.wantSet[t] {
			c.wantSet[t] = true
			c.wanted = append(c.wanted, t)
		}
	}
	c.waiting[id] = reply
	if unsubscribe {
		c.unsubscribes[id] = true
	}
	conn := c.conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.waiting, id)
		delete(c.unsubscribes, id)
		c.mu.Unlock()
	}()
	verb, frame := "subscribe", encodeSubscribe
	if unsubscribe {
		verb, frame = "unsubscribe", encodeUnsubscribe
	}
	if conn == nil {
		// Between connections: the next connection subscribes what is wanted
		// then, which these topics are, or no longer are.
		return nil, &sdkerrors.RealtimeError{Msg: "market stream is reconnecting; the " + verb + " applies once it is connected"}
	}
	if err := c.write(conn, frame(id, topics)); err != nil {
		return nil, &sdkerrors.RealtimeError{Msg: "market stream " + verb + ": " + err.Error() + "; it applies once the client reconnects"}
	}
	select {
	case out := <-reply:
		return out.resp, out.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, ErrClosed
	}
}

// OrderBook returns a copy of a pair's depth20 or depth book, up to levels
// per side (all when levels is 0); false before its first snapshot.
func (c *Client) OrderBook(symbolID uint32, stream Stream, levels int) (OrderBook, bool) {
	c.state.RLock()
	defer c.state.RUnlock()
	b := c.books[Topic{SymbolID: symbolID, Stream: stream}]
	if b == nil || !b.has {
		return OrderBook{}, false
	}
	scale, _ := c.cfg.Pairs.BaseQuantityScaleForSymbolID(symbolID)
	symbol := c.symbol(symbolID)
	limit := func(n int) int {
		if levels > 0 {
			return min(n, levels)
		}
		return n
	}
	return OrderBook{
		SymbolID: symbolID, Symbol: symbol, Stream: stream,
		BookEpoch: b.epoch, UpdateID: b.updateID, LevelsPerSide: b.window,
		Bids:  levelsOf(b.bids[:limit(len(b.bids))], scale, symbol, symbolID),
		Asks:  levelsOf(b.asks[:limit(len(b.asks))], scale, symbol, symbolID),
		Stale: !b.synced || c.feeds[symbolID] == FeedStale,
	}, true
}

// BestBidOffer returns a pair's latest best bid and offer; false before the
// first.
func (c *Client) BestBidOffer(symbolID uint32) (BestBidOffer, bool) {
	c.state.RLock()
	defer c.state.RUnlock()
	q := c.quotes[symbolID]
	if q == nil {
		return BestBidOffer{}, false
	}
	return *q, true
}

// Feed returns a pair's feed state as last reported; FeedLive unless the
// server reported it stale.
func (c *Client) Feed(symbolID uint32) FeedState {
	c.state.RLock()
	defer c.state.RUnlock()
	if f, ok := c.feeds[symbolID]; ok {
		return f
	}
	return FeedLive
}

func (c *Client) symbol(symbolID uint32) string {
	if s := c.cfg.Pairs.SymbolForSymbolID(symbolID); s != nil {
		return *s
	}
	return ""
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	header := http.Header{"User-Agent": []string{useragent.String()}}
	conn, resp, err := c.cfg.Dialer.DialContext(ctx, c.cfg.URL, header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		msg := "market stream connect: " + redact.URLError(err).Error()
		if resp != nil {
			msg += fmt.Sprintf(" (HTTP %d)", resp.StatusCode)
		}
		return nil, &sdkerrors.RealtimeError{Msg: msg}
	}
	conn.SetReadLimit(maxMessageBytes)
	return conn, nil
}

func (c *Client) write(conn *websocket.Conn, frame []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

// run serves connections until the client stops.
func (c *Client) run(conn *websocket.Conn, wanted []Topic) {
	defer close(c.done)
	defer func() {
		c.backfills.Wait()
		c.mu.Lock()
		for id, w := range c.waiting {
			w <- subscribeOutcome{err: ErrClosed}
			delete(c.waiting, id)
		}
		c.mu.Unlock()
		close(c.events)
	}()
	var delay backoff
	reconnect := false
	for {
		err := c.session(conn, wanted, reconnect)
		if c.ctx.Err() != nil {
			return
		}
		var schema *SchemaError
		if errors.As(err, &schema) {
			c.fail(err)
			return
		}
		if c.Err() != nil {
			return
		}
		if c.healthy.Swap(false) {
			delay.reset()
		}
		for {
			wait := delay.next()
			if !c.emit(&Disconnected{Err: err, Delay: wait}) {
				return
			}
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(wait):
			}
			conn, err = c.dial(c.ctx)
			if err == nil {
				var ok bool
				if wanted, ok = c.attach(conn); !ok {
					return
				}
				break
			}
			if c.ctx.Err() != nil {
				return
			}
		}
		reconnect = true
	}
}

// attach makes conn the client's connection and returns the topics to
// subscribe on it, atomically: a Subscribe either adds its topics before, and
// they are subscribed with these, or sends them on conn itself. False when
// the client is closed.
func (c *Client) attach(conn *websocket.Conn) ([]Topic, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = conn.Close()
		return nil, false
	}
	c.conn = conn
	c.ping.id = 0
	return append([]Topic(nil), c.wanted...), true
}

// session serves one attached connection until it fails.
func (c *Client) session(conn *websocket.Conn, wanted []Topic, reconnect bool) error {
	defer func() {
		_ = conn.Close()
		c.mu.Lock()
		c.conn = nil
		for id, w := range c.waiting {
			w <- subscribeOutcome{err: &sdkerrors.RealtimeError{Msg: "market stream connection lost before the subscribe response; the topics are subscribed once it reconnects"}}
			delete(c.waiting, id)
		}
		c.mu.Unlock()
	}()

	// A new connection holds nothing yet: books wait for their snapshot and
	// trades for their baseline.
	c.state.Lock()
	c.active = map[Topic]bool{}
	for _, b := range c.books {
		b.synced = false
	}
	for _, t := range c.trackers {
		t.restart()
	}
	clear(c.feeds)
	c.state.Unlock()

	stop := make(chan struct{})
	defer close(stop)
	pingErr := make(chan error, 1)
	go c.heartbeat(conn, stop, pingErr)

	if !c.emit(&Connected{Reconnect: reconnect}) {
		return nil
	}
	for start := 0; start < len(wanted); start += maxSubscribeItems {
		chunk := wanted[start:min(start+maxSubscribeItems, len(wanted))]
		if err := c.write(conn, encodeSubscribe(c.ids.Add(1), chunk)); err != nil {
			return &sdkerrors.RealtimeError{Msg: "market stream resubscribe: " + err.Error()}
		}
	}
	for {
		// The heartbeat detects a dead connection; the deadline only backs
		// it up.
		_ = conn.SetReadDeadline(time.Now().Add(2 * (c.cfg.PingInterval + c.cfg.PongTimeout)))
		typ, frame, err := conn.ReadMessage()
		if err != nil {
			select {
			case perr := <-pingErr:
				return perr
			default:
			}
			if c.ctx.Err() != nil {
				return nil
			}
			return readError(err)
		}
		if typ != websocket.BinaryMessage {
			return &FrameError{Msg: "text message on the binary market stream"}
		}
		if err := c.handle(frame); err != nil {
			return err
		}
	}
}

func readError(err error) error {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return &sdkerrors.RealtimeError{Msg: "market stream closed by the server: " + closeErr.Error(), CloseCode: closeErr.Code}
	}
	return &sdkerrors.RealtimeError{Msg: "market stream read: " + err.Error()}
}

// heartbeat pings every PingInterval and fails the connection when a pong is
// late.
func (c *Client) heartbeat(conn *websocket.Conn, stop <-chan struct{}, failed chan<- error) {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()
	check := time.NewTicker(max(min(c.cfg.PongTimeout/4, time.Second), time.Millisecond))
	defer check.Stop()
	for {
		select {
		case <-stop:
			return
		case <-check.C:
			c.mu.Lock()
			late := c.ping.id != 0 && time.Since(c.ping.sent) > c.cfg.PongTimeout
			c.mu.Unlock()
			if late {
				failed <- &sdkerrors.RealtimeError{Msg: fmt.Sprintf("market stream pong not received within %s", c.cfg.PongTimeout)}
				_ = conn.Close()
				return
			}
		case <-ticker.C:
			id := c.ids.Add(1)
			c.mu.Lock()
			if c.ping.id == 0 {
				c.ping.id, c.ping.sent = id, time.Now()
			} else {
				id = 0 // the previous ping is still owed its pong
			}
			c.mu.Unlock()
			if id != 0 {
				if err := c.write(conn, encodePing(id)); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}
}

// emit queues an event. A full queue fails the client: the consumer is too
// slow, and dropping messages would silently corrupt books and trades.
func (c *Client) emit(e Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil || c.closed {
		return false
	}
	select {
	case c.events <- e:
		return true
	default:
		c.err = &sdkerrors.QueueOverflowError{Msg: "market stream event queue full; consumer too slow"}
		c.cancel()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		return false
	}
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil && !c.closed {
		c.err = err
	}
	c.mu.Unlock()
	c.cancel()
}

func timeOf(ns uint64) time.Time {
	if ns > math.MaxInt64 {
		return time.Time{}
	}
	return time.Unix(0, int64(ns)).UTC()
}

func levelsOf(levels []levelMsg, scale int, symbol string, symbolID uint32) []Level {
	out := make([]Level, len(levels))
	for i, l := range levels {
		out[i] = Level{
			Price: codecs.DecodePriceTicks(l.price, symbol),
			Qty:   codecs.DecodeQtyScaled(l.qty, scale, symbol, &symbolID),
		}
	}
	return out
}

// backoff spaces reconnects: a delay drawn from the upper half of a cap that
// doubles from 500 milliseconds to 30 seconds, so clients do not reconnect
// in lockstep.
type backoff struct{ cap time.Duration }

const (
	backoffFirst = 500 * time.Millisecond
	backoffMax   = 30 * time.Second
)

func (b *backoff) reset() { b.cap = backoffFirst }

func (b *backoff) next() time.Duration {
	if b.cap <= 0 {
		b.reset()
	}
	half := b.cap / 2
	var raw [8]byte
	n := uint64(time.Now().UnixNano())
	if _, err := cryptorand.Read(raw[:]); err == nil {
		n = binary.LittleEndian.Uint64(raw[:])
	}
	delay := half + time.Duration(n%uint64(half+1))
	b.cap = min(b.cap*2, backoffMax)
	return delay
}
