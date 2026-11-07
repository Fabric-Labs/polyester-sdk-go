package realtime

import (
	"context"
	"sync"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/gorilla/websocket"
)

type renewalKind uint8

const (
	renewConnection renewalKind = iota
	renewSubscription
)

// tokenRenewer renews the connection and subscription tokens of one private
// Centrifugo connection before they expire. It takes fresh tokens from the
// same endpoints, and handles a failed renewal, as the private order stream
// does: the failure is reported, and the server ending the subscription or
// the connection then drives the usual resubscribe. Its work stops with the
// connection: a renewal is only ever sent on the connection that scheduled it.
type tokenRenewer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *websocket.Conn
	channel string
	// unit is the length of one TTL unit: a second, shortened in tests.
	unit  time.Duration
	fetch func(ctx context.Context, kind renewalKind) (string, error)
	// report receives renewal failures; the subscription stays open.
	report func(error)

	// writeMu serialises writes to conn: the reader answers pings while a
	// renewal sends its command.
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  uint32
	pending map[uint32]renewalKind
	timers  map[renewalKind]*time.Timer
	stopped bool
	wg      sync.WaitGroup
}

func newTokenRenewer(
	ctx context.Context,
	conn *websocket.Conn,
	channel string,
	unit time.Duration,
	firstID uint32,
	fetch func(ctx context.Context, kind renewalKind) (string, error),
	report func(error),
) *tokenRenewer {
	ctx, cancel := context.WithCancel(ctx)
	return &tokenRenewer{
		ctx:     ctx,
		cancel:  cancel,
		conn:    conn,
		channel: channel,
		unit:    unit,
		fetch:   fetch,
		report:  report,
		nextID:  firstID,
		pending: map[uint32]renewalKind{},
		timers:  map[renewalKind]*time.Timer{},
	}
}

// renewalWriteTimeout bounds one write, so a stalled socket cannot hold up
// the connection's shutdown, which waits for a renewal in flight.
const renewalWriteTimeout = 10 * time.Second

// write sends one frame; safe alongside a renewal.
func (r *tokenRenewer) write(frame []byte) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_ = r.conn.SetWriteDeadline(time.Now().Add(renewalWriteTimeout))
	return r.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// schedule renews kind's token ttl units after answered, when the reply
// that carried the TTL arrived, if the server said it expires. Time spent
// since then, such as the rest of the setup, counts against the TTL.
func (r *tokenRenewer) schedule(kind renewalKind, answered time.Time, expires bool, ttl uint32) {
	if !expires {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	if previous := r.timers[kind]; previous != nil && previous.Stop() {
		r.wg.Done()
	}
	r.wg.Add(1)
	delay := max(time.Until(answered.Add(time.Duration(ttl)*r.unit)), 0)
	r.timers[kind] = time.AfterFunc(delay, func() {
		defer r.wg.Done()
		r.renew(kind)
	})
}

func (r *tokenRenewer) renew(kind renewalKind) {
	token, err := r.fetch(r.ctx, kind)
	if r.ctx.Err() != nil {
		return
	}
	if err != nil {
		r.report(err)
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	id := r.nextID
	r.nextID++
	r.pending[id] = kind
	r.mu.Unlock()
	frame := refreshCommand(id, token)
	if kind == renewSubscription {
		frame = subRefreshCommand(id, r.channel, token)
	}
	// A failed write leaves the reader to see the connection end.
	_ = r.write(frame)
}

// handleReply takes the answer to a renewal and schedules the next one.
// It reports false for a reply that answers no renewal.
func (r *tokenRenewer) handleReply(message incomingMessage) bool {
	r.mu.Lock()
	kind, ok := r.pending[message.id]
	delete(r.pending, message.id)
	r.mu.Unlock()
	if !ok {
		return false
	}
	if message.err != nil {
		// The server ends what it no longer accepts; that drives recovery.
		r.report(renewalRefused(kind, message.err))
		return true
	}
	r.schedule(kind, time.Now(), message.expires, message.ttl)
	return true
}

func renewalRefused(kind renewalKind, protocolErr *protocolError) error {
	what := "connection"
	if kind == renewSubscription {
		what = "subscription"
	}
	err := centrifugoProtocolError(protocolErr).(*sdkerrors.RealtimeError)
	err.Msg = "realtime " + what + " token renewal refused: " + err.Msg
	return err
}

// stop cancels pending renewals and waits for one in flight to finish.
func (r *tokenRenewer) stop() {
	r.cancel()
	r.mu.Lock()
	r.stopped = true
	for _, timer := range r.timers {
		if timer.Stop() {
			r.wg.Done()
		}
	}
	r.mu.Unlock()
	r.wg.Wait()
}
