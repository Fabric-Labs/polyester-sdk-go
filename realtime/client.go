package realtime

import (
	"context"
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

// Client connects to Centrifugo for public and private protobuf channels.
type Client struct {
	wsURL       string
	apiURL      string
	credentials *auth.Credentials
	http        *http.Client
	maxQueue    int
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
		defer sub.Close()
		defer close(sub.ch)
		defer sendReady(&sdkerrors.RealtimeError{Msg: "realtime subscription ended before handshake"})
		for {
			if runCtx.Err() != nil {
				return
			}
			err := runSubscriptionOnce(runCtx, c, channel, decode, sub, func() {
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
			// Match Rust: first handshake failure is terminal (no silent reconnect).
			if !readySent.Load() {
				sendReady(err)
				return
			}
			if !reconnect {
				return
			}
			select {
			case <-runCtx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()

	select {
	case err := <-readyCh:
		if err != nil {
			cancel()
			return nil, err
		}
		return sub, nil
	case <-ctx.Done():
		cancel()
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
			return &sdkerrors.RealtimeError{Msg: err.Error()}
		}
		if messageType != websocket.BinaryMessage {
			return &sdkerrors.RealtimeError{Msg: "received JSON text frame on protobuf websocket"}
		}
		incoming, err := decodeReplies(raw)
		if err != nil {
			return &sdkerrors.RealtimeError{Msg: err.Error()}
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
					return &sdkerrors.RealtimeError{Msg: err.Error()}
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
	maxBuffered           int

	mu         sync.Mutex
	ready      bool
	disposed   bool
	generation int
	pending    []TPublication
	cancel     context.CancelFunc
	done       chan struct{}
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
	MaxBuffered       int
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
		maxBuffered:           maxBuffered,
		done:                  make(chan struct{}),
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
	return s.RefreshSnapshot(ctx)
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) run(ctx context.Context) {
	defer close(s.done)
	first := true
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
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}

		if !first {
			if s.onReconnect != nil {
				s.onReconnect()
			}
			_ = s.RefreshSnapshot(ctx)
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
		case <-time.After(time.Second):
		}
	}
}

func (s *SnapshotThenStream[TSnapshot, TPublication]) handlePublication(msg TPublication) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disposed {
		return
	}
	items := s.readPublication(msg)
	if !s.ready {
		if len(s.pending) >= s.maxBuffered {
			s.pending = s.pending[1:]
		}
		s.pending = append(s.pending, items...)
		return
	}
	s.applyLivePublications(items)
}

// RefreshSnapshot fetches a REST snapshot and merges buffered publications.
func (s *SnapshotThenStream[TSnapshot, TPublication]) RefreshSnapshot(ctx context.Context) error {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil
	}
	generation := s.generation + 1
	s.generation = generation
	s.ready = false
	s.pending = nil
	s.mu.Unlock()

	snapshot, err := s.fetchSnapshot(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.disposed || generation != s.generation {
		s.mu.Unlock()
		return nil
	}
	buffered := s.pending
	s.pending = nil
	s.applySnapshot(snapshot, buffered)
	s.ready = true
	onRefresh := s.onSnapshotRefresh
	s.mu.Unlock()
	if onRefresh != nil {
		onRefresh()
	}
	return nil
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
	}
	<-s.done
}

// IsReady reports whether the initial snapshot has been applied.
func (s *SnapshotThenStream[TSnapshot, TPublication]) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}
