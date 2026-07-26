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

// Client connects to Centrifugo for public and private protobuf channels.
type Client struct {
	wsURL       string
	apiURL      string
	credentials *auth.Credentials
	http        *http.Client
	maxQueue    int

	mu     sync.Mutex
	subs   []func()
	closed bool
}

// NewClient creates a realtime client.
func NewClient(wsURL string, apiURL string, credentials *auth.Credentials, httpClient *http.Client, maxQueue int) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		wsURL:       normalizeWSURL(wsURL),
		apiURL:      apiURL,
		credentials: credentials,
		http:        httpClient,
		maxQueue:    maxQueue,
	}
}

// Close cancels all tracked subscriptions.
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

func (c *Client) trackSubscription(closeFn func()) (untrack func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return func() {}
	}
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

	runCtx, cancel := context.WithCancel(ctx)
	sub := newSubscription[T](c.maxQueue, cancel)
	untrack := c.trackSubscription(func() { sub.Close() })
	sub.mu.Lock()
	sub.untrack = untrack
	sub.mu.Unlock()
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
		for {
			if runCtx.Err() != nil {
				return
			}
			err := runSubscriptionOnce(runCtx, c, channel, decode, sub, func() {
				backoff.reset()
				n := handshakes.Add(1)
				sub.noteHandshakeReady(n == 1)
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
			if errors.As(err, &terminal) {
				sub.fail(err)
				return
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
	decode func([]byte) (T, error),
	sub *Subscription[T],
	onReady func(),
) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     []string{centrifugoProtobufSubprotocol},
	}
	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return &sdkerrors.RealtimeError{Msg: err.Error()}
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

	if isPrivateChannel(channel) {
		token, err := fetchConnectionToken(ctx, c.http, c.credentials, c.apiURL)
		if err != nil {
			return err
		}
		if err := centrifugoConnect(ctx, conn, token); err != nil {
			return err
		}
		subToken, err := fetchSubscriptionToken(ctx, c.http, c.credentials, c.apiURL, channel)
		if err != nil {
			return err
		}
		if err := centrifugoSubscribe(ctx, conn, channel, subToken); err != nil {
			return err
		}
	} else {
		if err := centrifugoConnect(ctx, conn, ""); err != nil {
			return err
		}
		if err := centrifugoSubscribe(ctx, conn, channel, ""); err != nil {
			return err
		}
	}
	if onReady != nil {
		onReady()
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
		incoming, err := decodeReplies(raw)
		if err != nil {
			return terminalRealtime("invalid realtime protobuf frame: " + err.Error())
		}
		for _, message := range incoming {
			switch message.kind {
			case incomingPing:
				if err := conn.WriteMessage(websocket.BinaryMessage, pongCommand()); err != nil {
					return &sdkerrors.RealtimeError{Msg: err.Error()}
				}
			case incomingPublication:
				item, err := decode(message.data)
				if err != nil {
					return terminalRealtime("invalid realtime publication: " + err.Error())
				}
				if !sub.enqueue(item) {
					if err := sub.Err(); err != nil {
						return err
					}
					return ctx.Err()
				}
			case incomingReply:
				if message.err != nil {
					return centrifugoProtocolError(message.err)
				}
			}
		}
	}
}

func centrifugoConnect(ctx context.Context, conn *websocket.Conn, token string) error {
	if err := conn.WriteMessage(websocket.BinaryMessage, connectCommand(1, token)); err != nil {
		return &sdkerrors.RealtimeError{Msg: err.Error()}
	}
	return readCentrifugoReply(ctx, conn, 1)
}

func centrifugoSubscribe(ctx context.Context, conn *websocket.Conn, channel, token string) error {
	if err := conn.WriteMessage(websocket.BinaryMessage, subscribeCommand(2, channel, token)); err != nil {
		return &sdkerrors.RealtimeError{Msg: err.Error()}
	}
	return readCentrifugoReply(ctx, conn, 2)
}

func readCentrifugoReply(ctx context.Context, conn *websocket.Conn, expectedID uint32) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	_ = conn.SetReadDeadline(deadline)
	for {
		messageType, raw, err := conn.ReadMessage()
		if err != nil {
			return &sdkerrors.RealtimeError{Msg: err.Error()}
		}
		if messageType != websocket.BinaryMessage {
			return &sdkerrors.RealtimeError{Msg: "received JSON text reply on protobuf websocket"}
		}
		incoming, err := decodeReplies(raw)
		if err != nil {
			return &sdkerrors.RealtimeError{Msg: err.Error()}
		}
		for _, message := range incoming {
			switch {
			case message.kind == incomingPing:
				if err := conn.WriteMessage(websocket.BinaryMessage, pongCommand()); err != nil {
					return &sdkerrors.RealtimeError{Msg: err.Error()}
				}
			case message.kind == incomingReply && message.id == expectedID && message.err != nil:
				return centrifugoProtocolError(message.err)
			case message.kind == incomingReply && message.id == expectedID:
				return nil
			}
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
				callSnapshotNotification(s.onReconnect)
			}
			// Drain publications into the pending buffer while refresh runs
			// (ready=false). Without this, reconnect pubs sit in sub.Messages()
			// until after ready=true and incorrectly apply as live duplicates.
			refreshDone := make(chan error, 1)
			go func() {
				refreshDone <- s.refreshSnapshotWithRetry(ctx)
			}()
		drainRefresh:
			for {
				select {
				case <-ctx.Done():
					sub.Close()
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

	items, err := callSnapshotRead(s.readPublication, msg)
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
			s.disposed = true
			s.generation++
			s.lastErr = err
			s.mu.Unlock()
			callErrorCallback(callback, err)
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
	if err := callSnapshotApplyLive(s.applyLivePublications, items); err != nil {
		s.failClosed(err)
	}
}

// RefreshSnapshot fetches a REST snapshot and merges buffered publications.
// On failure the stream is marked not-ready and Err() is set. Success clears Err().
func (s *SnapshotThenStream[TSnapshot, TPublication]) RefreshSnapshot(ctx context.Context) error {
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

	snapshot, err := callSnapshotFetch(ctx, s.fetchSnapshot)
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
	if err := callSnapshotApply(s.applySnapshot, snapshot, buffered); err != nil {
		s.failClosed(err)
		return err
	}

	s.mu.Lock()
	if s.disposed || generation != s.generation {
		s.mu.Unlock()
		return nil
	}
	s.ready = true
	s.lastErr = nil
	onRefresh := s.onSnapshotRefresh
	s.mu.Unlock()
	callSnapshotNotification(onRefresh)
	return nil
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
	s.lastErr = err
	s.ready = false
	callback := s.onError
	s.mu.Unlock()
	callErrorCallback(callback, err)
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) failClosed(err error) {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.lastErr = err
	s.ready = false
	s.disposed = true
	s.generation++
	s.pending = nil
	callback := s.onError
	cancel := s.cancel
	s.mu.Unlock()
	callErrorCallback(callback, err)
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
	callErrorCallback(callback, err)
}

// Close stops the stream.
func (s *SnapshotThenStream[TSnapshot, TPublication]) Close() {
	s.mu.Lock()
	s.disposed = true
	s.generation++
	s.pending = nil
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-s.done
	}
}

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
