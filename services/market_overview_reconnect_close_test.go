package services

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	mosub "github.com/Fabric-Labs/polyester-sdk-go/marketoverview"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// reconnectHarness serves a market overview whose websocket can be dropped on
// demand and whose second snapshot fetch, the reconnect's, blocks until
// released and then answers or fails.
type reconnectHarness struct {
	svc          *MarketOverviewService
	drop         chan struct{}
	connected    chan int32
	fetchStarted chan struct{}
	releaseFetch chan struct{}
}

func newReconnectHarness(t *testing.T, failReconnectFetch bool) *reconnectHarness {
	t.Helper()
	h := &reconnectHarness{
		drop:         make(chan struct{}),
		connected:    make(chan int32, 4),
		fetchStarted: make(chan struct{}, 1),
		releaseFetch: make(chan struct{}),
	}
	conns := new(atomic.Int32)
	stop := make(chan struct{})
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	ws := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
			if err = conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		h.connected <- conns.Add(1)
		peerClosed := make(chan struct{})
		go func() {
			defer close(peerClosed)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		select {
		case <-h.drop:
		case <-peerClosed:
		case <-stop:
		}
	}))
	fetches := new(atomic.Int32)
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fetches.Add(1) > 1 {
			h.fetchStarted <- struct{}{}
			select {
			case <-h.releaseFetch:
			case <-r.Context().Done():
				return
			}
			if failReconnectFetch {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":"unavailable","message":"snapshot unavailable"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/proto")
		body, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, LastPriceTicks: int64(fetches.Load())}}})
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	factory := transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil)
	h.svc = NewMarketOverviewService(factory, catalogs.NewManager(), rt)
	return h
}

func (h *reconnectHarness) await(t *testing.T, what string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s never happened", what)
	}
}

func (h *reconnectHarness) awaitConnection(t *testing.T, want int32) {
	t.Helper()
	select {
	case got := <-h.connected:
		if got != want {
			t.Fatalf("connection %d, want %d", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("connection %d never opened", want)
	}
}

// POLY-9018 parity: a callback that runs on the reconnect's snapshot refresh,
// while the stream worker waits for that refresh because the replacement
// socket closed too, can close the subscription. Close returns at once, the
// worker exits, and no callback starts after Close returns.
//
// Schedule: connect; drop the socket; the reconnect's snapshot fetch blocks;
// drop the replacement socket, so the worker waits for the refresh; release
// the fetch; its callback (OnEvent from the applied snapshot, or OnError from
// the failed fetch) calls Close.
func TestClosingOverviewFromAReconnectRefreshCallbackReturnsAtOnce(t *testing.T) {
	for _, mode := range []string{"snapshot_event", "snapshot_error"} {
		t.Run(mode, func(t *testing.T) {
			h := newReconnectHarness(t, mode == "snapshot_error")
			var (
				handle    atomic.Pointer[mosub.Subscription]
				armed     atomic.Bool
				mu        sync.Mutex
				closed    bool
				late      []string
				returned  = make(chan time.Duration, 1)
				closeOnce sync.Once
			)
			closeFromCallback := func(kind string) {
				mu.Lock()
				if closed {
					late = append(late, kind)
				}
				mu.Unlock()
				if !armed.Load() {
					return
				}
				closeOnce.Do(func() {
					started := time.Now()
					handle.Load().Close()
					mu.Lock()
					closed = true
					mu.Unlock()
					returned <- time.Since(started)
				})
			}
			opts := MarketOverviewCreateSubscriptionOptions{
				OnEvent: func([]models.MarketOverviewEntry) { closeFromCallback("event") },
				OnError: func(error) {},
			}
			if mode == "snapshot_error" {
				opts.OnEvent = func([]models.MarketOverviewEntry) {
					mu.Lock()
					if closed {
						late = append(late, "event")
					}
					mu.Unlock()
				}
				opts.OnError = func(error) { closeFromCallback("error") }
			}
			sub, err := h.svc.CreateSubscription(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			handle.Store(sub)
			h.awaitConnection(t, 1)

			h.drop <- struct{}{}
			h.awaitConnection(t, 2)
			h.await(t, "the reconnect's snapshot fetch", h.fetchStarted)
			armed.Store(true)
			h.drop <- struct{}{}
			// Let the worker see the replacement socket close and wait for the refresh.
			time.Sleep(200 * time.Millisecond)
			close(h.releaseFetch)

			select {
			case took := <-returned:
				if took > promptClose {
					t.Fatalf("Close from the refresh callback took %v", took)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Close from the reconnect refresh callback blocked")
			}
			again := make(chan struct{})
			go func() { sub.Close(); close(again) }()
			select {
			case <-again:
			case <-time.After(5 * time.Second):
				t.Fatal("the stream worker never exited after Close")
			}
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			if len(late) != 0 {
				t.Fatalf("callbacks after Close returned: %v", late)
			}
		})
	}
}

// POLY-9018 parity: a Close from another goroutine while a reconnect's
// snapshot refresh is running its callback waits for that callback, so no
// callback is still running once Close returns.
func TestClosingOverviewDuringAReconnectRefreshCallbackWaitsForIt(t *testing.T) {
	h := newReconnectHarness(t, false)
	var (
		armed     atomic.Bool
		inside    = make(chan struct{})
		unblock   = make(chan struct{})
		running   atomic.Bool
		blockOnce sync.Once
	)
	sub, err := h.svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func([]models.MarketOverviewEntry) {
			if !armed.Load() {
				return
			}
			blockOnce.Do(func() {
				running.Store(true)
				close(inside)
				<-unblock
				running.Store(false)
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.awaitConnection(t, 1)
	h.drop <- struct{}{}
	h.awaitConnection(t, 2)
	h.await(t, "the reconnect's snapshot fetch", h.fetchStarted)
	armed.Store(true)
	close(h.releaseFetch)
	h.await(t, "the refresh callback", inside)

	closed := make(chan bool, 1)
	go func() {
		sub.Close()
		closed <- running.Load()
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while the refresh callback was still running")
	case <-time.After(300 * time.Millisecond):
	}
	close(unblock)
	select {
	case stillRunning := <-closed:
		if stillRunning {
			t.Fatal("Close returned while the refresh callback was still running")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close never returned after the refresh callback did")
	}
}
