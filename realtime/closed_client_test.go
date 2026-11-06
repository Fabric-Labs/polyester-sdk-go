package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/gorilla/websocket"
)

func TestProtoSubscribeAfterClose(t *testing.T) {
	var connects atomic.Int32
	upgrader := websocket.Upgrader{Subprotocols: []string{centrifugoProtobufSubprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		connects.Add(1)
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), "", nil, nil, 10)
	defer client.Close()
	decode := func(b []byte) (string, error) { return string(b), nil }
	control, err := SubscribeProto(ctx, client, "public:test:proto", decode)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if connects.Load() != 1 {
		t.Fatal("normal subscription never connected")
	}
	select {
	case <-control.Done():
		t.Fatal("normal subscription ended before close")
	default:
	}
	client.Close()
	select {
	case <-control.Done():
	default:
		t.Fatal("Close did not stop normal subscription")
	}
	calls := map[string]func() (*Subscription[string], error){
		"single": func() (*Subscription[string], error) { return SubscribeProto(ctx, client, "public:test:proto", decode) },
		"batch": func() (*Subscription[string], error) {
			return SubscribeProtoBatch(ctx, client, "public:test:proto", func(b []byte) ([]string, error) { return []string{string(b)}, nil })
		},
		"options": func() (*Subscription[string], error) {
			return SubscribeProtoWithOptions(ctx, client, "public:test:proto", decode, &SubscribeProtoOptions{})
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			sub, err := call()
			if sub != nil {
				defer sub.Close()
			}
			client.Close()
			var realtimeErr *sdkerrors.RealtimeError
			if sub != nil || !errors.Is(err, ErrClientClosed) || !errors.As(err, &realtimeErr) {
				t.Fatalf("after Close: subscription=%v error=%v, want ErrClientClosed", sub != nil, err)
			}
			if connects.Load() != 1 {
				t.Fatalf("closed client opened %d connections", connects.Load()-1)
			}
		})
	}
}

func TestProtoCloseDuringHandshake(t *testing.T) {
	reached := make(chan struct{})
	peerClosed := make(chan struct{})
	upgrader := websocket.Upgrader{Subprotocols: []string{centrifugoProtobufSubprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		close(reached)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(peerClosed)
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), "", nil, nil, 10)
	defer client.Close()
	result := make(chan error, 1)
	go func() {
		sub, err := SubscribeProto(ctx, client, "public:test:proto", func(b []byte) (string, error) { return string(b), nil })
		if sub != nil {
			sub.Close()
		}
		result <- err
	}()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("handshake never started")
	}
	client.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("unfinished handshake succeeded after close")
		}
	case <-ctx.Done():
		t.Fatal("subscribe did not return")
	}
	select {
	case <-peerClosed:
	case <-ctx.Done():
		t.Fatal("Close left handshake connection open")
	}
	client.Close()
}

type registeringContext struct {
	once sync.Once
	context.Context
	reached chan struct{}
	release chan struct{}
}

func (c *registeringContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.reached) })
	<-c.release
	return nil
}

func TestProtoCloseWhileRegistering(t *testing.T) {
	ctx := &registeringContext{Context: context.Background(), reached: make(chan struct{}), release: make(chan struct{})}
	client := NewClient("ws://127.0.0.1:1", "", nil, nil, 10)
	result := make(chan *Subscription[string], 1)
	go func() {
		sub, _ := SubscribeProto(ctx, client, "public:test:proto", func(b []byte) (string, error) { return string(b), nil })
		result <- sub
	}()
	select {
	case <-ctx.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("registration never started")
	}
	closed := make(chan struct{})
	go func() { client.Close(); close(closed) }()
	select {
	case <-closed:
		close(ctx.release)
		t.Fatal("Close passed subscription registration")
	case <-time.After(50 * time.Millisecond):
	}
	close(ctx.release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish")
	}
	select {
	case sub := <-result:
		if sub != nil {
			defer sub.Close()
			select {
			case <-sub.Done():
			default:
				t.Fatal("concurrent subscription escaped Close")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeProto did not finish")
	}
}
