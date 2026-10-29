package realtime

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// privateWSSubprotocol selects binary protobuf frames on /v1/ws/private.
const privateWSSubprotocol = "polyester.v1.proto"

// privateOrdersChannelPrefix names the channel the subscribe token is minted
// for, followed by the account's base58 public ID.
const privateOrdersChannelPrefix = "private:spot:orders:"

// closeHeartbeatTimeout is the close code sent when the server went silent.
const closeHeartbeatTimeout = 4000

// privateStreamTiming holds the protocol's timing; tests shorten it.
type privateStreamTiming struct {
	// heartbeatTimeout drops a connection that sent nothing for this long.
	heartbeatTimeout time.Duration
	// backoffBase and backoffMax bound reconnect and resubscribe delays.
	backoffBase time.Duration
	backoffMax  time.Duration
	// stableAfter resets the reconnect backoff for a connection up this long.
	stableAfter  time.Duration
	writeTimeout time.Duration
	dialTimeout  time.Duration
}

var defaultPrivateStreamTiming = privateStreamTiming{
	heartbeatTimeout: 3 * time.Second,
	backoffBase:      500 * time.Millisecond,
	backoffMax:       10 * time.Second,
	stableAfter:      10 * time.Second,
	writeTimeout:     5 * time.Second,
	dialTimeout:      10 * time.Second,
}

type privateStreamConfig struct {
	url                 string
	fetchConnectToken   func(ctx context.Context) (string, error)
	fetchSubscribeToken func(ctx context.Context, channel string) (string, error)
	maxQueue            int
	timing              privateStreamTiming
}

// streamSession spans one run of the stream, from its first subscription to
// the departure of its last. Every goroutine and timer of the run belongs to
// it, so a consumer's Close can wait for them all.
type streamSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type privateConn struct {
	ws *websocket.Conn
}

type sessionTimer struct {
	timer *time.Timer
	sess  *streamSession
}

// stop cancels the timer; must hold the stream lock.
func (t *sessionTimer) stop() {
	if t != nil && t.timer.Stop() {
		t.sess.wg.Done()
	}
}

type orderConsumer struct {
	sub     *Subscription[OrderStreamEvent]
	sess    *streamSession
	stopCtx func() bool
}

type subState int

const (
	subPending subState = iota
	subConnected
	subDisconnected
)

// orderStreamSub is one server subscription, shared by every consumer of
// its account.
type orderStreamSub struct {
	consumers map[*orderConsumer]struct{}
	channel   string
	account   uint64
	// sub is the server subscription ID; empty until Subscribed.
	sub string
	seq uint64
	// cursor is the last applied resume position, kept across resubscribes
	// and reconnects.
	cursor  string
	staging bool
	staged  []*orderv1.Order
	// orders is the live view as last applied, for consumers that join
	// later; nil before the first snapshot.
	orders      map[uint64]models.Order
	state       subState
	stale       bool
	staleReason string
	// staleOrderIDs are the orders an order_unavailable names.
	staleOrderIDs []string
	// ready: the server subscription reached the account's read boundary
	// sampled at subscribe. Reset with every new server subscription.
	ready bool
	// retries counts subscribe attempts since the subscription last
	// delivered data; it drives the resubscribe backoff.
	retries    int
	retryTimer *sessionTimer
}

type requestKind int

const (
	requestSubscribe requestKind = iota
	requestRefresh
)

type pendingRequest struct {
	kind requestKind
	s    *orderStreamSub
}

// privateOrderStream carries every account order subscription of a client
// over one private WebSocket. All state is guarded by mu; the reader, token
// fetches and timers take it for each step. Consumers are only ever sent
// to through their bounded queues, never called, so no consumer code runs
// under the lock or on a stream goroutine.
type privateOrderStream struct {
	cfg privateStreamConfig

	mu               sync.Mutex
	subs             map[string]*orderStreamSub
	pending          map[string]pendingRequest
	bySub            map[string]*orderStreamSub
	unsubscribing    map[string]struct{}
	sess             *streamSession
	conn             *privateConn
	connecting       bool
	authSent         bool
	authSentAt       time.Time
	reconnectAttempt int
	reconnectTimer   *sessionTimer
	requestSeq       uint64
}

func newPrivateOrderStream(cfg privateStreamConfig) *privateOrderStream {
	if cfg.maxQueue <= 0 {
		cfg.maxQueue = 1000
	}
	return &privateOrderStream{
		cfg:           cfg,
		subs:          map[string]*orderStreamSub{},
		pending:       map[string]pendingRequest{},
		bySub:         map[string]*orderStreamSub{},
		unsubscribing: map[string]struct{}{},
	}
}

func (p *privateOrderStream) subscribe(ctx context.Context, account uint64) *Subscription[OrderStreamEvent] {
	channel := privateOrdersChannelPrefix + codecs.FormatID(account)
	sub := newSubscription[OrderStreamEvent](p.cfg.maxQueue, nil)
	c := &orderConsumer{sub: sub}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sess == nil {
		sessCtx, cancel := context.WithCancel(context.Background())
		p.sess = &streamSession{ctx: sessCtx, cancel: cancel}
	}
	c.sess = p.sess
	s, existing := p.subs[channel]
	if !existing {
		s = &orderStreamSub{
			consumers: map[*orderConsumer]struct{}{},
			channel:   channel,
			account:   account,
		}
		p.subs[channel] = s
	}
	s.consumers[c] = struct{}{}
	sub.cancel = func() { p.remove(s, c) }
	c.stopCtx = context.AfterFunc(ctx, sub.Close)
	if existing {
		p.catchUp(s, c)
	} else if p.conn != nil && p.authSent {
		p.subscribeServer(p.conn, s)
	} else if p.conn == nil && !p.connecting && p.reconnectTimer == nil {
		p.open()
	}
	return sub
}

// catchUp gives a consumer that joined a running subscription what the
// others already know.
func (p *privateOrderStream) catchUp(s *orderStreamSub, c *orderConsumer) {
	if s.state == subConnected {
		p.offer(s, c, OrderStreamConnected{})
	}
	if s.orders != nil {
		p.offer(s, c, OrderSnapshot{Orders: sortedOrders(s.orders)})
	}
	if s.ready {
		p.offer(s, c, OrderReady{})
	}
	if s.stale {
		p.offer(s, c, OrderStale{Stale: true, Reason: s.staleReason, OrderIDs: s.staleOrderIDs})
	}
}

func sortedOrders(orders map[uint64]models.Order) []models.Order {
	ids := make([]uint64, 0, len(orders))
	for id := range orders {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]models.Order, 0, len(ids))
	for _, id := range ids {
		out = append(out, orders[id])
	}
	return out
}

// remove drops a consumer; the last one ends the server subscription, and
// the connection when no account is left. When the run ended, it waits for
// the run's goroutines to stop.
func (p *privateOrderStream) remove(s *orderStreamSub, c *orderConsumer) {
	p.mu.Lock()
	if _, ok := s.consumers[c]; ok {
		delete(s.consumers, c)
		p.finish(c, nil)
		if len(s.consumers) == 0 {
			p.teardown(s)
		}
	}
	p.mu.Unlock()
	if c.sess.ctx.Err() != nil {
		c.sess.wg.Wait()
	}
}

// finish closes a consumer's queue; the consumer must already be out of its
// subscription's set, so nothing sends to it any more.
func (p *privateOrderStream) finish(c *orderConsumer, err error) {
	sub := c.sub
	sub.mu.Lock()
	if err != nil {
		sub.failLocked(err)
	}
	sub.closed = true
	sub.mu.Unlock()
	close(sub.ch)
	sub.markFinished()
	if c.stopCtx != nil {
		c.stopCtx()
	}
}

// offer queues an event for one consumer. A consumer whose queue is full
// fails with QueueOverflowError instead of losing events.
func (p *privateOrderStream) offer(s *orderStreamSub, c *orderConsumer, event OrderStreamEvent) {
	sub := c.sub
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	select {
	case sub.ch <- event:
		sub.mu.Unlock()
		return
	default:
	}
	callback := sub.onError
	sub.mu.Unlock()
	err := &sdkerrors.QueueOverflowError{Msg: "private order stream queue full; consumer too slow"}
	delete(s.consumers, c)
	p.finish(c, err)
	notify(callback, err)
	if len(s.consumers) == 0 {
		p.teardown(s)
	}
}

func (p *privateOrderStream) deliver(s *orderStreamSub, event OrderStreamEvent) {
	for c := range s.consumers {
		p.offer(s, c, event)
	}
}

// report delivers a non-terminal error to a subscription's consumers.
func (p *privateOrderStream) report(s *orderStreamSub, err error) {
	for c := range s.consumers {
		c.sub.mu.Lock()
		callback := c.sub.onError
		c.sub.mu.Unlock()
		p.offer(s, c, OrderStreamError{Err: err})
		notify(callback, err)
	}
}

func (p *privateOrderStream) reportAll(err error) {
	for _, s := range p.snapshotSubs() {
		p.report(s, err)
	}
}

// notify runs a SetOnError callback off the stream's goroutines.
func notify(callback func(error), err error) {
	if callback != nil {
		go callErrorCallback(callback, err)
	}
}

func (p *privateOrderStream) snapshotSubs() []*orderStreamSub {
	out := make([]*orderStreamSub, 0, len(p.subs))
	for _, s := range p.subs {
		out = append(out, s)
	}
	return out
}

func (p *privateOrderStream) isCurrent(s *orderStreamSub) bool {
	return p.subs[s.channel] == s
}

// goSession runs f on a goroutine of the current run; nothing once the run
// ended.
func (p *privateOrderStream) goSession(f func(ctx context.Context)) {
	sess := p.sess
	if sess == nil {
		return
	}
	sess.wg.Add(1)
	go func() {
		defer sess.wg.Done()
		f(sess.ctx)
	}()
}

// afterFunc runs f under the stream lock after d, as part of the current
// run; nothing once the run ended.
func (p *privateOrderStream) afterFunc(d time.Duration, f func(t *sessionTimer)) *sessionTimer {
	sess := p.sess
	if sess == nil {
		return nil
	}
	t := &sessionTimer{sess: sess}
	sess.wg.Add(1)
	t.timer = time.AfterFunc(d, func() {
		defer sess.wg.Done()
		p.mu.Lock()
		defer p.mu.Unlock()
		f(t)
	})
	return t
}

// backoff is 0.5 s doubling to 10 s, jittered over the upper half of each step.
func (p *privateOrderStream) backoff(attempt int) time.Duration {
	ceiling := p.cfg.timing.backoffMax
	if attempt < 30 {
		ceiling = min(ceiling, p.cfg.timing.backoffBase<<(attempt+1))
	}
	half := ceiling / 2
	return half + rand.N(half+1)
}

// open fetches the connect token first: the server closes a socket that
// sends no auth within 5 seconds.
func (p *privateOrderStream) open() {
	sess := p.sess
	p.connecting = true
	p.goSession(func(ctx context.Context) {
		token, err := p.cfg.fetchConnectToken(ctx)
		tokenFailed := err != nil
		var ws *websocket.Conn
		if err == nil {
			ws, err = p.dial(ctx)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.sess != sess {
			if ws != nil {
				_ = ws.Close()
			}
			return
		}
		p.connecting = false
		if err != nil {
			if tokenFailed && isTerminalTokenError(err) {
				p.terminateAll(err)
				return
			}
			p.reportAll(err)
			p.scheduleReconnect()
			return
		}
		conn := &privateConn{ws: ws}
		p.conn = conn
		p.authenticate(conn, token)
		p.goSession(func(context.Context) { p.read(conn) })
	})
}

func (p *privateOrderStream) dial(ctx context.Context) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: p.cfg.timing.dialTimeout,
		Subprotocols:     []string{privateWSSubprotocol},
	}
	ws, _, err := dialer.DialContext(ctx, p.cfg.url, nil)
	if err != nil {
		return nil, &sdkerrors.RealtimeError{Msg: "private order stream: " + err.Error()}
	}
	// The server defaults to protobuf frames when it echoes no subprotocol.
	if sp := ws.Subprotocol(); sp != "" && sp != privateWSSubprotocol {
		_ = ws.Close()
		return nil, &sdkerrors.RealtimeError{Msg: "private order stream: server chose subprotocol " + sp}
	}
	ws.SetReadLimit(MaxRealtimeMessageBytes)
	return ws, nil
}

func isTerminalTokenError(err error) bool {
	var authErr *sdkerrors.AuthError
	return errors.As(err, &authErr)
}

func (p *privateOrderStream) authenticate(conn *privateConn, token string) {
	p.send(conn, &privatewsv1.ClientMessage{Op: &privatewsv1.ClientMessage_Auth{Auth: &privatewsv1.Auth{Token: token}}})
	p.authSent = true
	p.authSentAt = time.Now()
	for _, s := range p.snapshotSubs() {
		p.subscribeServer(conn, s)
	}
}

func (p *privateOrderStream) reauthenticate(conn *privateConn) {
	p.goSession(func(ctx context.Context) {
		token, err := p.cfg.fetchConnectToken(ctx)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.conn != conn {
			return
		}
		if err != nil {
			if isTerminalTokenError(err) {
				p.terminateAll(err)
			} else {
				p.reportAll(err)
			}
			return
		}
		p.send(conn, &privatewsv1.ClientMessage{Op: &privatewsv1.ClientMessage_Auth{Auth: &privatewsv1.Auth{Token: token}}})
	})
}

func (p *privateOrderStream) subscribeServer(conn *privateConn, s *orderStreamSub) {
	if !p.isCurrent(s) {
		return
	}
	id := p.nextRequestID()
	p.pending[id] = pendingRequest{kind: requestSubscribe, s: s}
	p.goSession(func(ctx context.Context) {
		token, err := p.cfg.fetchSubscribeToken(ctx, s.channel)
		p.mu.Lock()
		defer p.mu.Unlock()
		if request, ok := p.pending[id]; !ok || request.s != s {
			return
		}
		if err != nil {
			delete(p.pending, id)
			if isTerminalTokenError(err) {
				p.terminate(s, err)
				return
			}
			p.report(s, err)
			p.retrySubscribe(conn, s)
			return
		}
		if p.conn != conn {
			return
		}
		p.send(conn, &privatewsv1.ClientMessage{Op: &privatewsv1.ClientMessage_Subscribe{Subscribe: &privatewsv1.Subscribe{
			Id:      id,
			Channel: privatewsv1.Channel_ORDERS,
			Account: s.account,
			Token:   token,
			Cursor:  s.cursor,
		}}})
	})
}

// retrySubscribe subscribes again after a backoff that grows until the
// subscription delivers data.
func (p *privateOrderStream) retrySubscribe(conn *privateConn, s *orderStreamSub) {
	p.clearRetry(s)
	delay := p.backoff(s.retries)
	s.retries++
	s.retryTimer = p.afterFunc(delay, func(t *sessionTimer) {
		if s.retryTimer != t {
			return
		}
		s.retryTimer = nil
		if p.conn == conn && p.isCurrent(s) && s.sub == "" {
			p.subscribeServer(conn, s)
		}
	})
}

func (p *privateOrderStream) clearRetry(s *orderStreamSub) {
	s.retryTimer.stop()
	s.retryTimer = nil
}

func (p *privateOrderStream) refresh(conn *privateConn, s *orderStreamSub) {
	serverSub := s.sub
	p.goSession(func(ctx context.Context) {
		token, err := p.cfg.fetchSubscribeToken(ctx, s.channel)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.conn != conn || s.sub != serverSub || !p.isCurrent(s) {
			return
		}
		if err != nil {
			// An unrefreshed subscription ends with TOKEN_EXPIRED and resubscribes.
			p.report(s, err)
			return
		}
		id := p.nextRequestID()
		p.pending[id] = pendingRequest{kind: requestRefresh, s: s}
		p.send(conn, &privatewsv1.ClientMessage{Op: &privatewsv1.ClientMessage_SubscribeRefresh{SubscribeRefresh: &privatewsv1.SubscribeRefresh{
			Id:    id,
			Sub:   serverSub,
			Token: token,
		}}})
	})
}

// resubscribe drops the server subscription and subscribes again from the
// last applied cursor: at once the first time, then with backoff until the
// subscription delivers data.
func (p *privateOrderStream) resubscribe(conn *privateConn, s *orderStreamSub) {
	if s.sub != "" {
		delete(p.bySub, s.sub)
		p.unsubscribe(conn, s.sub)
	}
	s.sub = ""
	s.seq = 0
	s.ready = false
	s.staging = false
	s.staged = nil
	if s.retries > 0 {
		p.retrySubscribe(conn, s)
	} else {
		s.retries = 1
		p.subscribeServer(conn, s)
	}
	// Consumers see the gap as a disconnect and the new Subscribed as a reconnect.
	if s.state == subConnected {
		s.state = subDisconnected
		p.deliver(s, OrderStreamDisconnected{})
	}
}

func (p *privateOrderStream) unsubscribe(conn *privateConn, serverSub string) {
	id := p.nextRequestID()
	p.unsubscribing[id] = struct{}{}
	p.send(conn, &privatewsv1.ClientMessage{Op: &privatewsv1.ClientMessage_Unsubscribe{Unsubscribe: &privatewsv1.Unsubscribe{
		Id:  id,
		Sub: serverSub,
	}}})
}

func (p *privateOrderStream) read(conn *privateConn) {
	for {
		_ = conn.ws.SetReadDeadline(time.Now().Add(p.cfg.timing.heartbeatTimeout))
		messageType, data, err := conn.ws.ReadMessage()
		if err != nil {
			code, reason := p.closeInfo(conn, err)
			p.mu.Lock()
			p.onClosed(conn, code, reason)
			p.mu.Unlock()
			return
		}
		p.mu.Lock()
		p.onFrame(conn, messageType, data)
		p.mu.Unlock()
	}
}

// closeInfo maps a read error to a close code. A connection silent past the
// heartbeat timeout is closed by the client with 4000.
func (p *privateOrderStream) closeInfo(conn *privateConn, err error) (int, string) {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return closeErr.Code, closeErr.Text
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		reason := "heartbeat timeout"
		_ = conn.ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(closeHeartbeatTimeout, reason),
			time.Now().Add(p.cfg.timing.writeTimeout),
		)
		return closeHeartbeatTimeout, reason
	}
	return websocket.CloseAbnormalClosure, err.Error()
}

func (p *privateOrderStream) onFrame(conn *privateConn, messageType int, data []byte) {
	if p.conn != conn {
		return
	}
	if messageType != websocket.BinaryMessage {
		p.reportAll(&sdkerrors.RealtimeError{Msg: "private order stream: expected a binary frame"})
		return
	}
	msg := &privatewsv1.ServerMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		p.reportAll(&sdkerrors.RealtimeError{Msg: "private order stream: invalid frame: " + err.Error()})
		return
	}

	switch frame := msg.GetFrame().(type) {
	case *privatewsv1.ServerMessage_Subscribed:
		request, ok := p.pending[frame.Subscribed.GetId()]
		if !ok || request.kind != requestSubscribe {
			// Its consumers left before the server confirmed it.
			if msg.GetSub() != "" {
				p.unsubscribe(conn, msg.GetSub())
			}
			return
		}
		delete(p.pending, frame.Subscribed.GetId())
		s := request.s
		s.sub = msg.GetSub()
		s.seq = 0
		s.ready = false
		p.bySub[s.sub] = s
		if s.state != subConnected {
			s.state = subConnected
			p.deliver(s, OrderStreamConnected{})
		}
		// A new subscription reports unavailable itself if its view is still behind.
		if s.stale {
			s.stale = false
			s.staleReason = ""
			s.staleOrderIDs = nil
			p.deliver(s, OrderStale{Stale: false})
		}
		return
	case *privatewsv1.ServerMessage_SubscribeRefreshed:
		delete(p.pending, frame.SubscribeRefreshed.GetId())
		return
	case *privatewsv1.ServerMessage_Unsubscribed:
		if _, ok := p.unsubscribing[frame.Unsubscribed.GetId()]; ok {
			delete(p.unsubscribing, frame.Unsubscribed.GetId())
			return
		}
	case *privatewsv1.ServerMessage_Error:
		p.onRequestError(conn, msg.GetSub(), frame.Error)
		return
	case *privatewsv1.ServerMessage_TokenExpiring:
		p.reauthenticate(conn)
		return
	}

	s := p.bySub[msg.GetSub()]
	if s == nil {
		return
	}
	// A frame kind this client does not know is skipped, but its seq counts.
	if seq := msg.GetSeq(); seq > 0 {
		if seq != s.seq+1 {
			p.resubscribe(conn, s)
			return
		}
		s.seq = seq
	}

	switch frame := msg.GetFrame().(type) {
	case *privatewsv1.ServerMessage_SnapshotBegin:
		s.staging = true
		s.staged = nil
	case *privatewsv1.ServerMessage_SnapshotPage:
		if s.staging {
			s.staged = append(s.staged, frame.SnapshotPage.GetOrders()...)
		}
	case *privatewsv1.ServerMessage_SnapshotEnd:
		staged, staging := s.staged, s.staging
		s.staging = false
		s.staged = nil
		if !staging || len(staged) != int(frame.SnapshotEnd.GetCount()) {
			s.cursor = ""
			p.resubscribe(conn, s)
			return
		}
		s.cursor = frame.SnapshotEnd.GetCursor()
		s.retries = 0
		orders := decode.OrdersFromProto(staged)
		s.orders = make(map[uint64]models.Order, len(orders))
		for i, order := range staged {
			s.orders[order.GetOrderId()] = orders[i]
		}
		p.deliver(s, OrderSnapshot{Orders: orders})
	case *privatewsv1.ServerMessage_Unit:
		unit := frame.Unit
		s.cursor = unit.GetNextCursor()
		s.retries = 0
		orders := decode.OrdersFromProto(unit.GetOrders())
		removed := make([]string, 0, len(unit.GetRemovedOrderIds()))
		if s.orders != nil {
			for i, order := range unit.GetOrders() {
				s.orders[order.GetOrderId()] = orders[i]
			}
		}
		for _, id := range unit.GetRemovedOrderIds() {
			if s.orders != nil {
				delete(s.orders, id)
			}
			removed = append(removed, codecs.FormatUint64ID(id))
		}
		p.deliver(s, OrderUnit{
			Orders:          orders,
			RemovedOrderIDs: removed,
			Events:          decode.OrderUnitEventsFromProto(unit.GetEvents(), unit.GetOrders()),
		})
	case *privatewsv1.ServerMessage_Covered:
		s.cursor = frame.Covered.GetCursor()
		s.retries = 0
	case *privatewsv1.ServerMessage_Ready:
		// A resumed subscription on a quiet account delivers no data, only this.
		s.retries = 0
		if !s.ready {
			s.ready = true
			p.deliver(s, OrderReady{})
		}
	case *privatewsv1.ServerMessage_ResyncRequired:
		// The snapshot that follows replaces the state; nothing in between is delivered.
		s.staging = false
		s.staged = nil
	case *privatewsv1.ServerMessage_Unavailable:
		// A later Unavailable replaces the earlier reason and order IDs.
		s.stale = true
		s.staleReason = unavailableReason(frame.Unavailable.GetReason())
		s.staleOrderIDs = nil
		for _, id := range frame.Unavailable.GetOrderIds() {
			s.staleOrderIDs = append(s.staleOrderIDs, codecs.FormatUint64ID(id))
		}
		p.deliver(s, OrderStale{Stale: true, Reason: s.staleReason, OrderIDs: s.staleOrderIDs})
	case *privatewsv1.ServerMessage_Available:
		s.stale = false
		s.staleReason = ""
		s.staleOrderIDs = nil
		p.deliver(s, OrderStale{Stale: false})
	case *privatewsv1.ServerMessage_SubscribeExpiring:
		p.refresh(conn, s)
	case *privatewsv1.ServerMessage_Unsubscribed:
		// The server ended it: its token expired, or for a reason this client does not know.
		delete(p.bySub, s.sub)
		s.sub = ""
		p.resubscribe(conn, s)
	}
}

func unavailableReason(reason privatewsv1.UnavailableReason) string {
	if reason == privatewsv1.UnavailableReason_UNAVAILABLE_REASON_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(reason.String())
}

func (p *privateOrderStream) onRequestError(conn *privateConn, serverSub string, requestErr *privatewsv1.RequestError) {
	id := requestErr.GetId()
	if id != "" {
		if _, ok := p.unsubscribing[id]; ok {
			delete(p.unsubscribing, id)
			return
		}
	}
	var request pendingRequest
	hasRequest := false
	if id != "" {
		request, hasRequest = p.pending[id]
		// The answer to a request already answered, or whose subscription has gone.
		if !hasRequest {
			return
		}
		delete(p.pending, id)
	}
	s := request.s
	if !hasRequest {
		s = p.bySub[serverSub]
	}
	code := requestErr.GetCode().String()
	message := requestErr.GetMessage()
	if message == "" {
		message = "private order stream request failed: " + code
	}
	err := &sdkerrors.RealtimeError{Msg: message, Code: code}
	if s == nil {
		p.reportAll(err)
		return
	}
	if hasRequest && request.kind == requestSubscribe {
		if requestErr.GetCode() == privatewsv1.RequestError_CURSOR_INVALID {
			s.cursor = ""
			p.subscribeServer(conn, s)
			return
		}
		p.terminate(s, err)
		return
	}
	if requestErr.GetCode() == privatewsv1.RequestError_SUBSCRIPTION_UNKNOWN && s.sub == serverSub {
		delete(p.bySub, s.sub)
		s.sub = ""
		p.resubscribe(conn, s)
		return
	}
	p.report(s, err)
}

func (p *privateOrderStream) onClosed(conn *privateConn, code int, reason string) {
	if p.conn != conn {
		return
	}
	stable := p.authSent && time.Since(p.authSentAt) >= p.cfg.timing.stableAfter
	p.detach()
	_ = conn.ws.Close()
	if stable {
		p.reconnectAttempt = 0
	}
	if len(p.subs) == 0 {
		return
	}
	// A malformed frame (4400) and a refused connect token (4403) recur on every reconnect.
	if code == 4400 || code == 4403 {
		if reason == "" {
			reason = "private WebSocket closed with code " + strconv.Itoa(code)
		}
		p.terminateAll(&sdkerrors.RealtimeError{Msg: reason, CloseCode: code})
		return
	}
	p.scheduleReconnect()
	// A close that reconnects is not an error.
	for _, s := range p.snapshotSubs() {
		if s.state == subConnected {
			s.state = subDisconnected
			p.deliver(s, OrderStreamDisconnected{})
		}
	}
}

func (p *privateOrderStream) scheduleReconnect() {
	delay := p.backoff(p.reconnectAttempt)
	p.reconnectAttempt++
	p.reconnectTimer = p.afterFunc(delay, func(t *sessionTimer) {
		if p.reconnectTimer != t {
			return
		}
		p.reconnectTimer = nil
		if len(p.subs) > 0 && p.conn == nil && !p.connecting {
			p.open()
		}
	})
}

// detach forgets the current connection and every server subscription on it.
func (p *privateOrderStream) detach() {
	p.conn = nil
	p.authSent = false
	clear(p.pending)
	clear(p.bySub)
	clear(p.unsubscribing)
	for _, s := range p.subs {
		p.clearRetry(s)
		s.sub = ""
		s.seq = 0
		s.ready = false
		s.staging = false
		s.staged = nil
	}
}

// shutdown ends the run: it closes the connection and cancels every
// goroutine and timer of the run.
func (p *privateOrderStream) shutdown() {
	conn := p.conn
	p.detach()
	p.reconnectTimer.stop()
	p.reconnectTimer = nil
	p.reconnectAttempt = 0
	p.connecting = false
	if p.sess != nil {
		p.sess.cancel()
		p.sess = nil
	}
	if conn != nil {
		_ = conn.ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(p.cfg.timing.writeTimeout),
		)
		_ = conn.ws.Close()
	}
}

func (p *privateOrderStream) teardown(s *orderStreamSub) {
	if !p.isCurrent(s) {
		return
	}
	delete(p.subs, s.channel)
	for c := range s.consumers {
		delete(s.consumers, c)
		p.finish(c, nil)
	}
	p.clearRetry(s)
	if p.conn != nil && s.sub != "" {
		delete(p.bySub, s.sub)
		p.unsubscribe(p.conn, s.sub)
	}
	s.sub = ""
	for id, request := range p.pending {
		if request.s == s {
			delete(p.pending, id)
		}
	}
	if len(p.subs) == 0 {
		p.shutdown()
	}
}

// terminate ends a subscription with err: its consumers get the error and a
// disconnect, then their queues close with Err set.
func (p *privateOrderStream) terminate(s *orderStreamSub, err error) {
	if !p.isCurrent(s) {
		return
	}
	p.report(s, err)
	if s.state != subDisconnected {
		s.state = subDisconnected
		p.deliver(s, OrderStreamDisconnected{})
	}
	for c := range s.consumers {
		delete(s.consumers, c)
		p.finish(c, err)
	}
	p.teardown(s)
}

func (p *privateOrderStream) terminateAll(err error) {
	for _, s := range p.snapshotSubs() {
		p.terminate(s, err)
	}
}

func (p *privateOrderStream) send(conn *privateConn, msg *privatewsv1.ClientMessage) {
	if p.conn != conn {
		return
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		p.reportAll(&sdkerrors.RealtimeError{Msg: fmt.Sprintf("private order stream: encode %T: %v", msg.GetOp(), err)})
		return
	}
	_ = conn.ws.SetWriteDeadline(time.Now().Add(p.cfg.timing.writeTimeout))
	if err := conn.ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
		// The reader sees the broken connection and reconnects.
		_ = conn.ws.Close()
	}
}

func (p *privateOrderStream) nextRequestID() string {
	p.requestSeq++
	return strconv.FormatUint(p.requestSeq, 10)
}
