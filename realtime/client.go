package realtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/callbackmark"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
	"github.com/gorilla/websocket"
)

const centrifugoProtobufSubprotocol = "centrifuge-protobuf"

// MaxRealtimeMessageBytes bounds inbound WebSocket buffering and protobuf records.
const MaxRealtimeMessageBytes int64 = 8 * 1024 * 1024

type terminalRealtimeError struct {
	err error
}

func (e *terminalRealtimeError) Error() string { return e.err.Error() }
func (e *terminalRealtimeError) Unwrap() error { return e.err }

func terminalRealtime(message string) error {
	return &terminalRealtimeError{err: &sdkerrors.RealtimeError{Msg: message}}
}

// Client connects to Centrifugo for public and private protobuf channels, and
// to the private WebSocket for account order streams (SubscribeOrders).
type Client struct {
	wsURL       string
	apiURL      string
	credentials *auth.Credentials
	http        *http.Client
	maxQueue    int
	// privateTiming is the private order stream's protocol timing.
	privateTiming privateStreamTiming
	// renewalUnit is the unit of a Centrifugo token TTL: a second; tests
	// shorten it.
	renewalUnit time.Duration
	// access decides whether a subscription the server ended may resume.
	access *subaccountAccess

	mu     sync.Mutex
	subs   []func()
	closed bool
	// orders carries every account order stream; created on first use.
	orders *privateOrderStream
}

// NewClient creates a realtime client.
func NewClient(wsURL string, apiURL string, credentials *auth.Credentials, httpClient *http.Client, maxQueue int) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	// Token requests are signed: they never follow a redirect.
	httpClient = useragent.WrapClient(auth.RefuseSignedRedirects(httpClient))
	c := &Client{
		wsURL:         normalizeWSURL(wsURL),
		apiURL:        apiURL,
		credentials:   credentials,
		http:          httpClient,
		maxQueue:      maxQueue,
		privateTiming: defaultPrivateStreamTiming,
		renewalUnit:   time.Second,
	}
	c.access = newSubaccountAccess(c, defaultResubscribeTiming)
	return c
}

// Close permanently closes the client and cancels all tracked subscriptions.
// Create a new Client to subscribe again.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	subs := c.subs
	c.subs = nil
	c.mu.Unlock()
	for _, closeFn := range subs {
		closeFn()
	}
}

// trackSubscriptionLocked registers closeFn for Close; c.mu must be held and
// the client open.
func (c *Client) trackSubscriptionLocked(closeFn func()) (untrack func()) {
	c.subs = append(c.subs, closeFn)
	idx := len(c.subs) - 1
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if idx >= 0 && idx < len(c.subs) {
			c.subs[idx] = func() {}
		}
	}
}

func normalizeWSURL(wsURL string) string {
	url := strings.TrimRight(wsURL, "/")
	if strings.HasSuffix(url, "/connection/websocket") {
		return url
	}
	return url + "/connection/websocket"
}

func isPrivateChannel(channel string) bool {
	return strings.HasPrefix(channel, "private:")
}

// SubscribeProtoOptions configures protobuf subscription behavior.
type SubscribeProtoOptions struct {
	// AutoReconnect controls whether the transport reconnects after disconnect.
	// Default true for raw subscriptions. Snapshot-then-stream sets this false so
	// it can refresh REST state between reconnect attempts.
	AutoReconnect *bool
}

func autoReconnectEnabled(opts *SubscribeProtoOptions) bool {
	if opts == nil || opts.AutoReconnect == nil {
		return true
	}
	return *opts.AutoReconnect
}

// SubscribeProto subscribes to a protobuf Centrifugo channel.
func SubscribeProto[T any](ctx context.Context, c *Client, channel string, decode func([]byte) (T, error)) (*Subscription[T], error) {
	return SubscribeProtoWithOptions(ctx, c, channel, decode, nil)
}

// SubscribeProtoBatch subscribes to a protobuf Centrifugo channel whose
// publications each carry a batch of events. decode returns the events of one
// publication in order; each arrives as its own message. A publication that
// does not decode fails the subscription, as with SubscribeProto.
func SubscribeProtoBatch[T any](ctx context.Context, c *Client, channel string, decode func([]byte) ([]T, error)) (*Subscription[T], error) {
	return subscribeProto(ctx, c, channel, decode, nil)
}

// SubscribeProtoWithOptions subscribes with optional reconnect control.
//
// It waits for the Centrifugo connect/subscribe handshake (including private
// token fetch) to succeed before returning. Initial auth/handshake failures are
// returned immediately and do not reconnect in the background.
func SubscribeProtoWithOptions[T any](
	ctx context.Context,
	c *Client,
	channel string,
	decode func([]byte) (T, error),
	opts *SubscribeProtoOptions,
) (*Subscription[T], error) {
	return subscribeProto(ctx, c, channel, func(payload []byte) ([]T, error) {
		item, err := decode(payload)
		if err != nil {
			return nil, err
		}
		return []T{item}, nil
	}, opts)
}

func subscribeProto[T any](
	ctx context.Context,
	c *Client,
	channel string,
	decode func([]byte) ([]T, error),
	opts *SubscribeProtoOptions,
) (*Subscription[T], error) {
	if c == nil {
		return nil, &sdkerrors.RealtimeError{Msg: "Realtime client is not configured"}
	}
	if isPrivateChannel(channel) {
		if c.credentials == nil {
			return nil, &sdkerrors.AuthError{Msg: fmt.Sprintf(`Cannot subscribe to private channel "%s" without API-key credentials`, channel)}
		}
		if c.apiURL == "" {
			return nil, &sdkerrors.RealtimeError{Msg: "Realtime private channels require api_url"}
		}
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClientClosed
	}
	runCtx, cancel := context.WithCancel(ctx)
	sub := newSubscription[T](c.maxQueue, cancel)
	// Register while holding the lock Close takes, so it cannot miss this sub.
	sub.untrack = c.trackSubscriptionLocked(func() { sub.Close() })
	c.mu.Unlock()
	reconnect := autoReconnectEnabled(opts)
	readyCh := make(chan error, 1)
	var readySent atomic.Bool
	var handshakes atomic.Uint64
	sendReady := func(err error) {
		if readySent.CompareAndSwap(false, true) {
			readyCh <- err
		}
	}

	go func() {
		defer close(sub.ch)
		defer sub.markFinished()
		defer sendReady(&sdkerrors.RealtimeError{Msg: "realtime subscription ended before handshake"})
		var backoff reconnectBackoff
		var resume *resubscribeAttempt
		for {
			if runCtx.Err() != nil {
				return
			}
			err := runSubscriptionOnce(runCtx, c, channel, decode, sub, func() {
				backoff.reset()
				n := handshakes.Add(1)
				sub.noteHandshakeReady(n == 1)
				if resume != nil {
					resume.noteReady(time.Now())
				}
				sendReady(nil)
			})
			if err == nil || runCtx.Err() != nil {
				return
			}
			// Overflow and other terminal subscription faults must not reconnect.
			if sub.Err() != nil {
				return
			}
			var terminal *terminalRealtimeError
			var redirected *sdkerrors.RedirectRefusedError
			if errors.As(err, &terminal) || errors.As(err, &redirected) {
				sub.fail(err)
				return
			}
			var ended *serverUnsubscribedError
			if errors.As(err, &ended) && !ended.temporary() {
				if !reconnect {
					sub.fail(ended.stop(channel, sdkerrors.SubscriptionEndNotResubscribable, nil))
					return
				}
				resume = c.access.attemptAfter(resume, ended, time.Now())
			}
			if resume != nil && !resume.ready {
				// The server ended this subscription: resume, or say why not.
				resume.note(err)
				var refused *sdkerrors.AuthError
				if errors.As(err, &refused) {
					sub.fail(resume.ended.stop(channel, sdkerrors.SubscriptionEndTokenRefused, err))
					return
				}
				if stop := c.access.awaitResume(runCtx, channel, resume); stop != nil {
					sub.fail(stop)
					return
				}
				continue
			}
			// Match Rust: first handshake failure is terminal (no silent reconnect).
			if !readySent.Load() {
				sendReady(err)
				return
			}
			if !reconnect {
				sub.fail(err)
				return
			}
			sub.notifyError(err)
			select {
			case <-runCtx.Done():
				return
			case <-time.After(backoff.next()):
			}
		}
	}()

	select {
	case err := <-readyCh:
		if err != nil {
			cancel()
			<-sub.done
			return nil, err
		}
		return sub, nil
	case <-ctx.Done():
		cancel()
		<-sub.done
		return nil, ctx.Err()
	}
}

func runSubscriptionOnce[T any](
	ctx context.Context,
	c *Client,
	channel string,
	decode func([]byte) ([]T, error),
	sub *Subscription[T],
	onReady func(),
) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     []string{centrifugoProtobufSubprotocol},
	}
	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return &sdkerrors.RealtimeError{Msg: redact.URLError(err).Error()}
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(MaxRealtimeMessageBytes)
	// Abort ReadMessage promptly when the subscription context is canceled.
	abortWatcherDone := make(chan struct{})
	defer close(abortWatcherDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-abortWatcherDone:
		}
	}()
	if conn.Subprotocol() != centrifugoProtobufSubprotocol {
		return &sdkerrors.RealtimeError{Msg: "server did not negotiate centrifuge-protobuf websocket subprotocol"}
	}

	var pending []incomingMessage
	write := func(frame []byte) error { return conn.WriteMessage(websocket.BinaryMessage, frame) }
	var renewer *tokenRenewer
	if isPrivateChannel(channel) {
		token, err := fetchConnectionToken(ctx, c.http, c.credentials, c.apiURL)
		if err != nil {
			return err
		}
		connected, err := centrifugoConnect(ctx, conn, token, &pending)
		if err != nil {
			return err
		}
		// The connection token's TTL runs from here, through the rest of setup.
		connectedAt := time.Now()
		subToken, err := fetchSubscriptionToken(ctx, c.http, c.credentials, c.apiURL, channel)
		if err != nil {
			return err
		}
		subscribed, err := centrifugoSubscribe(ctx, conn, channel, subToken, &pending)
		if err != nil {
			return err
		}
		subscribedAt := time.Now()
		renewer = newTokenRenewer(ctx, conn, channel, c.renewalUnit, 3, func(ctx context.Context, kind renewalKind) (string, error) {
			if kind == renewConnection {
				return fetchConnectionToken(ctx, c.http, c.credentials, c.apiURL)
			}
			return fetchSubscriptionToken(ctx, c.http, c.credentials, c.apiURL, channel)
		}, sub.notifyError)
		defer renewer.stop()
		write = renewer.write
		renewer.schedule(renewConnection, connectedAt, connected.expires, connected.ttl)
		renewer.schedule(renewSubscription, subscribedAt, subscribed.expires, subscribed.ttl)
	} else {
		if _, err := centrifugoConnect(ctx, conn, "", &pending); err != nil {
			return err
		}
		if _, err := centrifugoSubscribe(ctx, conn, channel, "", &pending); err != nil {
			return err
		}
	}
	if onReady != nil {
		onReady()
	}
	subaccountsChannel := strings.HasPrefix(channel, subaccountsChannelPrefix)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		incoming := pending
		pending = nil
		if len(incoming) == 0 {
			// Use a long read deadline so Centrifugo ping/pong can complete without
			// retrying ReadMessage on a deadline-poisoned connection (gorilla/websocket).
			_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			messageType, raw, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if strings.Contains(strings.ToLower(err.Error()), "read limit") ||
					websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
					return terminalRealtime(fmt.Sprintf(
						"realtime message exceeds %d bytes",
						MaxRealtimeMessageBytes,
					))
				}
				return &sdkerrors.RealtimeError{Msg: err.Error()}
			}
			if messageType != websocket.BinaryMessage {
				return terminalRealtime("received JSON text frame on protobuf websocket")
			}
			incoming, err = decodeReplies(raw)
			if err != nil {
				return terminalRealtime("invalid realtime protobuf frame: " + err.Error())
			}
		}
		for _, message := range incoming {
			switch message.kind {
			case incomingPing:
				if err := write(pongCommand()); err != nil {
					return &sdkerrors.RealtimeError{Msg: err.Error()}
				}
			case incomingPublication:
				if subaccountsChannel {
					c.access.noteUpdate(message.data)
				}
				items, err := decode(message.data)
				if err != nil {
					return terminalRealtime("invalid realtime publication: " + err.Error())
				}
				for _, item := range items {
					if !sub.enqueue(item) {
						if err := sub.Err(); err != nil {
							return err
						}
						return ctx.Err()
					}
				}
			case incomingUnsubscribe:
				if message.channel == "" || message.channel == channel {
					return &serverUnsubscribedError{code: message.code, reason: message.reason}
				}
			case incomingReply:
				if renewer != nil && renewer.handleReply(message) {
					continue
				}
				if message.err != nil {
					return centrifugoProtocolError(message.err)
				}
			}
		}
	}
}

func centrifugoConnect(ctx context.Context, conn *websocket.Conn, token string, pending *[]incomingMessage) (incomingMessage, error) {
	if err := conn.WriteMessage(websocket.BinaryMessage, connectCommand(1, token)); err != nil {
		return incomingMessage{}, &sdkerrors.RealtimeError{Msg: err.Error()}
	}
	return readCentrifugoReply(ctx, conn, 1, pending)
}

func centrifugoSubscribe(ctx context.Context, conn *websocket.Conn, channel, token string, pending *[]incomingMessage) (incomingMessage, error) {
	if err := conn.WriteMessage(websocket.BinaryMessage, subscribeCommand(2, channel, token)); err != nil {
		return incomingMessage{}, &sdkerrors.RealtimeError{Msg: err.Error()}
	}
	return readCentrifugoReply(ctx, conn, 2, pending)
}

// readCentrifugoReply returns the reply to expectedID, holding back anything
// else that arrives first.
func readCentrifugoReply(ctx context.Context, conn *websocket.Conn, expectedID uint32, pending *[]incomingMessage) (incomingMessage, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	_ = conn.SetReadDeadline(deadline)
	for {
		messageType, raw, err := conn.ReadMessage()
		if err != nil {
			return incomingMessage{}, &sdkerrors.RealtimeError{Msg: err.Error()}
		}
		if messageType != websocket.BinaryMessage {
			return incomingMessage{}, &sdkerrors.RealtimeError{Msg: "received JSON text reply on protobuf websocket"}
		}
		incoming, err := decodeReplies(raw)
		if err != nil {
			return incomingMessage{}, &sdkerrors.RealtimeError{Msg: err.Error()}
		}
		matched := false
		var reply incomingMessage
		for _, message := range incoming {
			switch {
			case message.kind == incomingPing:
				if err := conn.WriteMessage(websocket.BinaryMessage, pongCommand()); err != nil {
					return incomingMessage{}, &sdkerrors.RealtimeError{Msg: err.Error()}
				}
			case message.kind == incomingReply && message.id == expectedID && message.err != nil:
				return incomingMessage{}, centrifugoProtocolError(message.err)
			case message.kind == incomingReply && message.id == expectedID:
				matched, reply = true, message
			default:
				if len(*pending) >= 200 {
					return incomingMessage{}, &sdkerrors.QueueOverflowError{Msg: "realtime setup buffer full"}
				}
				*pending = append(*pending, message)
			}
		}
		if matched {
			return reply, nil
		}
	}
}

func centrifugoProtocolError(protocolErr *protocolError) error {
	temporary := ""
	if protocolErr.temporary {
		temporary = " (temporary)"
	}
	return &sdkerrors.RealtimeError{
		Msg: fmt.Sprintf("centrifugo error %d: %s%s", protocolErr.code, protocolErr.message, temporary),
	}
}

// SnapshotThenStream coordinates REST snapshot hydration with a live channel.
//
// Callbacks and shutdown. Every callback the stream calls (the config's
// FetchSnapshot, ReadPublication, ApplySnapshot, ApplyLivePublications,
// OnReconnect, OnSnapshotRefresh and OnError) runs inside runCallback, which
// records the calling goroutine as running a callback of this stream
// (internal/callbackmark); the managed orderbook and market-overview
// subscriptions mark the callbacks they call for it the same way. Close
// called from such a goroutine only signals shutdown and returns. Every other
// Close waits for the worker.
//
// Lock order: refreshSem, then mu, then the callbackmark lock.
//   - mu and the callbackmark lock are never held across a callback, a wait
//     on a channel or another lock acquisition, apart from this order.
//   - refreshSem is held for a whole refresh, its callbacks included. A refresh
//     acquires it in a select that also returns when the stream is stopped
//     (stopped closes) or its context ends, so no wait on it outlives Stop.
//   - Waits: Close waits on done, and run waits on refreshDone (the reconnect
//     refresh it started). Neither is ever made while holding refreshSem or mu,
//     or from inside a callback: a Close from a callback never waits. Once the
//     stream is stopped, a refresh that waits for refreshSem gives up, so a
//     chain of waits that reaches refreshSem ends when the stream stops.
type SnapshotThenStream[TSnapshot any, TPublication any] struct {
	client                *Client
	channel               string
	decode                func([]byte) (TPublication, error)
	fetchSnapshot         func(context.Context) (TSnapshot, error)
	readPublication       func(TPublication) []TPublication
	applySnapshot         func(TSnapshot, []TPublication)
	applyLivePublications func([]TPublication)
	onReconnect           func()
	onSnapshotRefresh     func()
	onError               func(error)
	maxBuffered           int

	mu          sync.Mutex
	ready       bool
	disposed    bool
	generation  int
	pending     []TPublication
	lastErr     error
	cancel      context.CancelFunc
	done        chan struct{}
	connected   chan error
	connectOnce sync.Once
	refreshRun  bool
	refreshNext bool
	// refreshSem serializes refreshes; see the lock order above.
	refreshSem chan struct{}
	// stopped closes when the stream is disposed: stopped or failed closed.
	stopped chan struct{}
}

// SnapshotThenStreamConfig configures snapshot-then-stream behavior.
type SnapshotThenStreamConfig[TSnapshot any, TPublication any] struct {
	Client                *Client
	Channel               string
	Decode                func([]byte) (TPublication, error)
	FetchSnapshot         func(context.Context) (TSnapshot, error)
	ReadPublication       func(TPublication) []TPublication
	ApplySnapshot         func(TSnapshot, []TPublication)
	ApplyLivePublications func([]TPublication)
	// OnReconnect is called after a transport disconnect before snapshot rebuild.
	OnReconnect func()
	// OnSnapshotRefresh is called after a successful snapshot rebuild.
	OnSnapshotRefresh func()
	// OnError is called when snapshot or stream processing fails.
	OnError     func(error)
	MaxBuffered int
}

func snapshotCallbackError(name string, recovered any) error {
	return &sdkerrors.RealtimeError{Msg: fmt.Sprintf("%s callback panicked: %v", name, recovered)}
}

func callSnapshotFetch[TSnapshot any](
	ctx context.Context,
	callback func(context.Context) (TSnapshot, error),
) (value TSnapshot, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = snapshotCallbackError("fetch_snapshot", recovered)
		}
	}()
	return callback(ctx)
}

func callSnapshotRead[TPublication any](
	callback func(TPublication) []TPublication,
	publication TPublication,
) (items []TPublication, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = snapshotCallbackError("read_publication", recovered)
		}
	}()
	return callback(publication), nil
}

func callSnapshotApply[TSnapshot, TPublication any](
	callback func(TSnapshot, []TPublication),
	snapshot TSnapshot,
	publications []TPublication,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = snapshotCallbackError("apply_snapshot", recovered)
		}
	}()
	callback(snapshot, publications)
	return nil
}

func callSnapshotApplyLive[TPublication any](
	callback func([]TPublication),
	publications []TPublication,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = snapshotCallbackError("apply_live_publications", recovered)
		}
	}()
	callback(publications)
	return nil
}

func callSnapshotNotification(callback func()) {
	if callback == nil {
		return
	}
	defer func() { _ = recover() }()
	callback()
}

// NewSnapshotThenStream creates a snapshot-then-stream coordinator.
func NewSnapshotThenStream[TSnapshot any, TPublication any](cfg SnapshotThenStreamConfig[TSnapshot, TPublication]) *SnapshotThenStream[TSnapshot, TPublication] {
	maxBuffered := cfg.MaxBuffered
	if maxBuffered <= 0 {
		maxBuffered = 200
	}
	return &SnapshotThenStream[TSnapshot, TPublication]{
		client:                cfg.Client,
		channel:               cfg.Channel,
		decode:                cfg.Decode,
		fetchSnapshot:         cfg.FetchSnapshot,
		readPublication:       cfg.ReadPublication,
		applySnapshot:         cfg.ApplySnapshot,
		applyLivePublications: cfg.ApplyLivePublications,
		onReconnect:           cfg.OnReconnect,
		onSnapshotRefresh:     cfg.OnSnapshotRefresh,
		onError:               cfg.OnError,
		maxBuffered:           maxBuffered,
		done:                  make(chan struct{}),
		connected:             make(chan error, 1),
		refreshSem:            make(chan struct{}, 1),
		stopped:               make(chan struct{}),
	}
}

// Start begins websocket streaming and performs the initial snapshot refresh.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go s.run(runCtx)
	select {
	case err := <-s.connected:
		if err != nil {
			s.Close()
			return err
		}
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	}
	if err := s.RefreshSnapshot(ctx); err != nil {
		s.Close()
		return err
	}
	return nil
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) signalInitialConnection(err error) {
	s.connectOnce.Do(func() {
		s.connected <- err
	})
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) run(ctx context.Context) {
	defer close(s.done)
	first := true
	var backoff reconnectBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		disposed := s.disposed
		s.mu.Unlock()
		if disposed {
			return
		}

		noReconnect := false
		sub, err := SubscribeProtoWithOptions(ctx, s.client, s.channel, s.decode, &SubscribeProtoOptions{
			AutoReconnect: &noReconnect,
		})
		if err != nil {
			if ctx.Err() != nil {
				s.signalInitialConnection(ctx.Err())
				return
			}
			s.setErr(err)
			if first {
				s.signalInitialConnection(err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff.next()):
				continue
			}
		}
		backoff.reset()
		if first {
			s.signalInitialConnection(nil)
		}

		if !first {
			if s.onReconnect != nil {
				s.runCallback(func() { callSnapshotNotification(s.onReconnect) })
			}
			// Drain publications into the pending buffer while refresh runs
			// (ready=false). Without this, reconnect pubs sit in sub.Messages()
			// until after ready=true and incorrectly apply as live duplicates.
			// run owns this refresh: it waits for it on every path, so done
			// never closes while one of its callbacks may still run. The wait
			// ends promptly once the stream stops (see the lock order).
			refreshDone := make(chan error, 1)
			go func() {
				refreshDone <- s.refreshSnapshotWithRetry(ctx)
			}()
		drainRefresh:
			for {
				select {
				case <-ctx.Done():
					sub.Close()
					<-refreshDone
					return
				case err := <-refreshDone:
					if err != nil {
						s.setErr(err)
						// Fail-closed: stop the stream after a failed reconnect refresh.
						sub.Close()
						return
					}
					break drainRefresh
				case <-sub.Done():
					err := <-refreshDone
					if err != nil {
						s.setErr(err)
						return
					}
					break drainRefresh
				case msg, ok := <-sub.Messages():
					if !ok {
						err := <-refreshDone
						if err != nil {
							s.setErr(err)
							return
						}
						break drainRefresh
					}
					s.handlePublication(msg)
				}
			}
		}
		first = false

		func() {
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case <-sub.Done():
					return
				case msg, ok := <-sub.Messages():
					if !ok {
						return
					}
					s.handlePublication(msg)
				}
			}
		}()
		if err := sub.Err(); err != nil {
			var terminal *terminalRealtimeError
			var overflow *sdkerrors.QueueOverflowError
			var ended *sdkerrors.SubscriptionEndedError
			if errors.As(err, &terminal) || errors.As(err, &overflow) || errors.As(err, &ended) {
				s.failClosed(err)
				return
			}
			s.setErr(err)
		}

		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		disposed = s.disposed
		s.mu.Unlock()
		if disposed {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff.next()):
		}
	}
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) handlePublication(msg TPublication) {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	var items []TPublication
	var err error
	s.runCallback(func() { items, err = callSnapshotRead(s.readPublication, msg) })
	if err != nil {
		s.failClosed(err)
		return
	}
	if len(items) == 0 {
		return
	}

	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	if !s.ready {
		if len(s.pending)+len(items) > s.maxBuffered {
			err := &sdkerrors.QueueOverflowError{Msg: "snapshot recovery buffer full; recreate the subscription"}
			callback := s.onError
			cancel := s.cancel
			s.pending = nil
			s.ready = false
			s.disposeLocked()
			s.generation++
			s.lastErr = err
			s.mu.Unlock()
			s.runCallback(func() { callErrorCallback(callback, err) })
			if cancel != nil {
				cancel()
			}
			return
		}
		s.pending = append(s.pending, items...)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if err := s.applyLive(items); err != nil {
		s.failClosed(err)
	}
}

// applyLive applies live publications as a callback.
func (s *SnapshotThenStream[TSnapshot, TPublication]) applyLive(items []TPublication) (err error) {
	s.runCallback(func() { err = callSnapshotApplyLive(s.applyLivePublications, items) })
	return err
}

// RefreshSnapshot fetches a REST snapshot and merges buffered publications.
// On failure the stream is marked not-ready and Err() is set. Success clears Err().
//
// Refreshes run one at a time. A refresh waiting for another to finish
// returns nil when the stream stops, and ctx's error when ctx ends.
func (s *SnapshotThenStream[TSnapshot, TPublication]) RefreshSnapshot(ctx context.Context) error {
	select {
	case s.refreshSem <- struct{}{}:
	case <-s.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.refreshSem }()
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil
	}
	generation := s.generation + 1
	s.generation = generation
	s.ready = false
	// Retain publications buffered during a failed attempt so a successful
	// retry merges them exactly once.
	s.mu.Unlock()

	var snapshot TSnapshot
	var err error
	s.runCallback(func() { snapshot, err = callSnapshotFetch(ctx, s.fetchSnapshot) })
	if err != nil {
		s.setErr(err)
		return err
	}

	s.mu.Lock()
	if s.disposed || generation != s.generation {
		s.mu.Unlock()
		return nil
	}
	buffered := s.pending
	s.pending = nil
	s.mu.Unlock()
	s.runCallback(func() { err = callSnapshotApply(s.applySnapshot, snapshot, buffered) })
	if err != nil {
		s.failClosed(err)
		return err
	}

	// Drain publications that arrived after pending was taken but before the
	// snapshot callback completed. Marking ready is atomic with observing an
	// empty pending queue, so no publication can be stranded pre-ready.
	for {
		s.mu.Lock()
		if s.disposed || generation != s.generation {
			s.mu.Unlock()
			return nil
		}
		buffered = s.pending
		s.pending = nil
		if len(buffered) == 0 {
			s.ready = true
			s.lastErr = nil
			onRefresh := s.onSnapshotRefresh
			s.mu.Unlock()
			s.runCallback(func() { callSnapshotNotification(onRefresh) })
			return nil
		}
		s.mu.Unlock()
		if err := s.applyLive(buffered); err != nil {
			s.failClosed(err)
			return err
		}
	}
}

// RequestSnapshotRefresh coalesces sequence-gap refresh requests into one
// bounded single-flight worker. A failed retry fails the stream closed.
func (s *SnapshotThenStream[TSnapshot, TPublication]) RequestSnapshotRefresh(ctx context.Context) {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.refreshNext = true
	if s.refreshRun {
		s.mu.Unlock()
		return
	}
	s.refreshRun = true
	s.mu.Unlock()

	go func() {
		for {
			s.mu.Lock()
			if s.disposed || !s.refreshNext {
				s.refreshRun = false
				s.mu.Unlock()
				return
			}
			s.refreshNext = false
			s.mu.Unlock()
			if err := s.refreshSnapshotWithRetry(ctx); err != nil {
				s.failClosed(err)
				return
			}
		}
	}()
}

// refreshSnapshotWithRetry performs one bounded retry then fail-closes.
func (s *SnapshotThenStream[TSnapshot, TPublication]) refreshSnapshotWithRetry(ctx context.Context) error {
	if err := s.RefreshSnapshot(ctx); err == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return s.RefreshSnapshot(ctx)
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) setErr(err error) {
	s.mu.Lock()
	if s.disposed {
		// Stopped or failed closed: what fails now is the shutdown itself,
		// and no callback may start after Close returns.
		s.mu.Unlock()
		return
	}
	s.lastErr = err
	s.ready = false
	callback := s.onError
	s.mu.Unlock()
	s.runCallback(func() { callErrorCallback(callback, err) })
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) failClosed(err error) {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.lastErr = err
	s.ready = false
	s.disposeLocked()
	s.generation++
	s.pending = nil
	callback := s.onError
	cancel := s.cancel
	s.mu.Unlock()
	s.runCallback(func() { callErrorCallback(callback, err) })
	if cancel != nil {
		cancel()
	}
}

// Err returns the last snapshot/stream error, if any.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// SetOnError installs a callback for transport, decode, snapshot, and terminal
// buffering errors. Callback panics are isolated from the stream worker.
func (s *SnapshotThenStream[TSnapshot, TPublication]) SetOnError(callback func(error)) {
	s.mu.Lock()
	s.onError = callback
	err := s.lastErr
	s.mu.Unlock()
	s.runCallback(func() { callErrorCallback(callback, err) })
}

// Stop requests shutdown without waiting for the worker. It is safe to call
// from any callback.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Stop() {
	s.mu.Lock()
	s.disposeLocked()
	s.generation++
	s.pending = nil
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// disposeLocked marks the stream disposed and releases every refresh waiting
// for refreshSem. s.mu must be held.
func (s *SnapshotThenStream[TSnapshot, TPublication]) disposeLocked() {
	if !s.disposed {
		s.disposed = true
		close(s.stopped)
	}
}

// Close stops the stream and waits for its worker to exit.
//
// Called from a callback of this stream, it only signals shutdown and
// returns: the callback may be running on the worker, on a refresh the worker
// waits for, or while holding the refresh lock another refresh waits for, so
// waiting could never end. The worker exits once the callback returns. Every
// other Close waits for the worker.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Close() {
	s.Stop()
	s.mu.Lock()
	started := s.cancel != nil
	s.mu.Unlock()
	if !started || s.inCallback() {
		return
	}
	<-s.done
}

// runCallback runs fn on the calling goroutine as a callback of this stream:
// a Close called from inside fn signals shutdown and returns without waiting.
func (s *SnapshotThenStream[TSnapshot, TPublication]) runCallback(fn func()) {
	callbackmark.Run(s, fn)
}

// inCallback reports whether the calling goroutine is running a callback of
// this stream.
func (s *SnapshotThenStream[TSnapshot, TPublication]) inCallback() bool {
	return callbackmark.Active(s)
}

// Done closes when the stream worker has permanently stopped.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Done() <-chan struct{} { return s.done }

// IsReady reports whether the initial snapshot has been applied.
func (s *SnapshotThenStream[TSnapshot, TPublication]) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}

// IsDisposed reports whether the coordinator has stopped permanently.
func (s *SnapshotThenStream[TSnapshot, TPublication]) IsDisposed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disposed
}
