package services

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// bookRefreshHarness serves an orderbook whose websockets can be dropped and
// fed deltas, and whose snapshot fetches follow a plan. Every snapshot
// answers the sequence the server last sent, so a delta that skips one is a
// gap the client refreshes for.
type bookRefreshHarness struct {
	svc          *OrderbookService
	conns        chan *sweepConn
	fetchStarted chan int32
	seq          atomic.Uint64
}

func newBookRefreshHarness(t *testing.T, plan func(n int32) fetchStep) *bookRefreshHarness {
	t.Helper()
	h := &bookRefreshHarness{conns: make(chan *sweepConn, 32), fetchStarted: make(chan int32, 64)}
	h.seq.Store(1)
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
		c := &sweepConn{frames: make(chan []byte, 16), drop: make(chan struct{})}
		h.conns <- c
		peerClosed := make(chan struct{})
		go func() {
			defer close(peerClosed)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case data := <-c.frames:
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			case <-c.drop:
				return
			case <-peerClosed:
				return
			case <-stop:
				return
			}
		}
	}))
	fetches := new(atomic.Int32)
	httpSrv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := fetches.Add(1)
		step := plan(n)
		h.fetchStarted <- n
		if step.gate != nil {
			select {
			case <-step.gate:
			case <-r.Context().Done():
				return
			case <-stop: // a failed test must not hang its cleanup
				return
			}
		}
		if step.delay > 0 {
			select {
			case <-time.After(step.delay):
			case <-r.Context().Done():
				return
			}
		}
		if step.fail {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"unavailable","message":"snapshot unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{SymbolId: 1, BookSeq: h.seq.Load(), Available: true})
		_, _ = w.Write(data)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": 1, "base_quantity_scale": 8},
	}}); err != nil {
		t.Fatal(err)
	}
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	cfg := transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}
	h.svc = NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt)
	return h
}

// delta is the next delta in sequence, starting where the last one ended, or,
// with gap, one that skips a view so the client waits for a reset.
func (h *bookRefreshHarness) delta(gap bool) []byte {
	start := h.seq.Load()
	if gap {
		start = h.seq.Add(1)
	}
	end := h.seq.Add(1)
	return bookTicketFrame(&ob.OrderBookDelta{BookSeqStart: start, BookSeqEnd: end})
}

func (h *bookRefreshHarness) nextConn(t *testing.T) *sweepConn {
	t.Helper()
	select {
	case c := <-h.conns:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("no websocket connection opened")
		return nil
	}
}

func (h *bookRefreshHarness) awaitFetch(t *testing.T, want int32) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case n := <-h.fetchStarted:
			if n == want {
				return
			}
		case <-deadline:
			t.Fatalf("snapshot fetch %d never started", want)
		}
	}
}

// A reconnect's snapshot refresh whose callback closes the subscription while
// a manual RefreshSnapshot waits for the refresh lock. Close returns at once,
// the manual refresh returns, and the worker exits. A sequence gap fetches no
// snapshot: it waits for the stream's next reset.
func TestClosingOrderbookFromARefreshCallbackWhileAManualRefreshWaits(t *testing.T) {
	gate := make(chan struct{})
	h := newBookRefreshHarness(t, func(n int32) fetchStep {
		if n == 2 {
			return fetchStep{gate: gate}
		}
		return fetchStep{}
	})
	var (
		handle   atomic.Pointer[orderbook.Subscription]
		armed    atomic.Bool
		once     sync.Once
		gaps     = make(chan struct{}, 4)
		returned = make(chan time.Duration, 1)
	)
	sub, err := h.svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{
		Symbol:        "BTC-USDT",
		OnSequenceGap: func() { gaps <- struct{}{} },
		OnEvent: func(models.OrderbookData) {
			if !armed.Load() {
				return
			}
			once.Do(func() {
				started := time.Now()
				handle.Load().Close()
				returned <- time.Since(started)
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Store(sub)
	conn := h.nextConn(t)

	conn.frames <- h.delta(true)
	select {
	case <-gaps:
	case <-time.After(10 * time.Second):
		t.Fatal("the sequence gap was never seen")
	}
	select {
	case n := <-h.fetchStarted:
		if n != 1 {
			t.Fatalf("the sequence gap fetched snapshot %d", n)
		}
	default:
	}
	close(conn.drop)
	h.awaitFetch(t, 2)
	manual := make(chan error, 1)
	go func() { manual <- sub.RefreshSnapshot(context.Background()) }()
	// Let the manual refresh start and wait for the refresh lock.
	time.Sleep(200 * time.Millisecond)
	armed.Store(true)
	close(gate)

	select {
	case took := <-returned:
		if took > promptClose {
			t.Fatalf("Close from the refresh callback took %v", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close from the refresh callback blocked")
	}
	select {
	case <-manual:
	case <-time.After(5 * time.Second):
		t.Fatal("the manual refresh never returned")
	}
	finished := make(chan struct{})
	go func() { sub.Close(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream worker never exited")
	}
}

// POLY-9018 parity: seeded schedules of sequence-gap refreshes, manual
// refreshes, reconnect refreshes after socket drops, in-order deltas, failing
// and slow snapshot fetches, and closes from OnEvent, from OnError and from
// another goroutine.
//
// Randomized per seed: each fetch's delay (up to 400ms, so refreshes
// overlap one another and a reconnect's backoff) and whether it fails; a
// sequence of up to eight actions (send an in-order delta, send a delta
// that skips a sequence, start a manual refresh, drop the socket, pause);
// and how the subscription is closed. Invariants: a Close from a callback
// returns at once; every Close, manual refresh and the worker finish; and no
// callback starts once every Close and manual refresh has returned. Replay
// one schedule with POLYESTER_INTERLEAVING_SEED=<seed>; run more with
// POLYESTER_INTERLEAVING_SEEDS=<count>.
func TestOrderbookRefreshAndCloseInterleavings(t *testing.T) {
	for _, seed := range closeScheduleSeeds(t) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) { runBookRefreshSchedule(t, seed) })
	}
}

func runBookRefreshSchedule(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 2))
	steps := make([]fetchStep, 64)
	for i := range steps {
		if i < 2 {
			continue // fetch 1 is the subscription's first snapshot
		}
		steps[i] = fetchStep{
			delay: []time.Duration{0, time.Millisecond, 20 * time.Millisecond, 150 * time.Millisecond, 400 * time.Millisecond}[rng.IntN(5)],
			fail:  rng.IntN(4) == 0,
		}
	}
	h := newBookRefreshHarness(t, func(n int32) fetchStep {
		if int(n) < len(steps) {
			return steps[n]
		}
		return fetchStep{}
	})
	closeMode := []string{"event", "error", "outside"}[rng.IntN(3)]
	closeAtEvent := int32(1 + rng.IntN(3))
	outsideDelay := []time.Duration{0, time.Millisecond, 50 * time.Millisecond, 400 * time.Millisecond, 900 * time.Millisecond}[rng.IntN(5)]

	var (
		handle      atomic.Pointer[orderbook.Subscription]
		armed       atomic.Bool
		events      atomic.Int32
		callbacks   atomic.Int64
		closeOnce   sync.Once
		mu          sync.Mutex
		slowCloses  []time.Duration
		manualCalls sync.WaitGroup
	)
	closeFromCallback := func() {
		closeOnce.Do(func() {
			started := time.Now()
			handle.Load().Close()
			mu.Lock()
			slowCloses = append(slowCloses, time.Since(started))
			mu.Unlock()
		})
	}
	sub, err := h.svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{
		Symbol: "BTC-USDT",
		OnEvent: func(models.OrderbookData) {
			callbacks.Add(1)
			if armed.Load() && closeMode == "event" && events.Add(1) == closeAtEvent {
				closeFromCallback()
			}
		},
		OnError: func(error) {
			callbacks.Add(1)
			if armed.Load() && closeMode == "error" {
				closeFromCallback()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Store(sub)
	conn := h.nextConn(t)
	armed.Store(true)

	outsideDone := make(chan struct{})
	if closeMode == "outside" {
		go func() {
			time.Sleep(outsideDelay)
			sub.Close()
			close(outsideDone)
		}()
	} else {
		close(outsideDone)
	}

	for range 1 + rng.IntN(8) {
		action := rng.IntN(5)
		if (action == 0 || action == 1) && conn == nil {
			select {
			case conn = <-h.conns:
			default:
			}
		}
		switch action {
		case 0, 1: // an in-order delta, or one that skips a view
			if conn != nil {
				select {
				case conn.frames <- h.delta(action == 1):
				default:
				}
			}
		case 2: // a manual refresh, as a caller would make
			manualCalls.Add(1)
			go func() {
				defer manualCalls.Done()
				_ = sub.RefreshSnapshot(context.Background())
			}()
		case 3: // drop the socket; the worker reconnects and refreshes
			if conn != nil {
				close(conn.drop)
				conn = nil
			}
		case 4:
			time.Sleep([]time.Duration{0, time.Millisecond, 30 * time.Millisecond, 300 * time.Millisecond, 600 * time.Millisecond}[rng.IntN(5)])
		}
	}

	finished := make(chan struct{})
	go func() {
		<-outsideDone
		sub.Close()
		manualCalls.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatalf("seed %d (%s close): a Close, a manual refresh or the worker never finished", seed, closeMode)
	}
	for range sub.Updates() {
	}
	quiet := callbacks.Load()
	time.Sleep(50 * time.Millisecond)
	if callbacks.Load() != quiet {
		t.Fatalf("seed %d: a callback started after every Close and refresh had returned", seed)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, took := range slowCloses {
		if took > promptClose {
			t.Fatalf("seed %d: a Close from a callback took %v", seed, took)
		}
	}
}
