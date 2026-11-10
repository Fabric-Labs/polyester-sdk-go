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
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	mosub "github.com/Fabric-Labs/polyester-sdk-go/marketoverview"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// fetchStep is how the market overview server answers one snapshot fetch.
type fetchStep struct {
	delay time.Duration
	fail  bool
	gate  <-chan struct{}
}

// sweepConn is one accepted websocket: write frames to it, or drop it.
type sweepConn struct {
	frames chan []byte
	drop   chan struct{}
}

// refreshHarness serves a market overview whose websockets can be dropped and
// fed publications, and whose snapshot fetches follow a plan.
type refreshHarness struct {
	svc          *MarketOverviewService
	conns        chan *sweepConn
	fetchStarted chan int32
}

func newRefreshHarness(t *testing.T, plan func(n int32) fetchStep) *refreshHarness {
	t.Helper()
	h := &refreshHarness{conns: make(chan *sweepConn, 32), fetchStarted: make(chan int32, 64)}
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
		body, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, LastPriceTicks: int64(n)}}})
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	factory := transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil)
	h.svc = NewMarketOverviewService(factory, catalogs.NewManager(), rt)
	return h
}

func (h *refreshHarness) nextConn(t *testing.T) *sweepConn {
	t.Helper()
	select {
	case c := <-h.conns:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("no websocket connection opened")
		return nil
	}
}

func (h *refreshHarness) awaitFetch(t *testing.T, want int32) {
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

// POLY-9018 parity: a manual RefreshSnapshot whose callback closes the
// subscription while a reconnect's refresh waits for the refresh lock.
//
// Schedule: subscribe; a manual refresh's fetch blocks, holding the refresh
// lock; drop the socket; the reconnect's refresh waits for the lock; release
// the manual fetch; its OnEvent calls Close. Close returns at once, the
// manual refresh returns, and the worker exits.
func TestClosingOverviewFromAManualRefreshCallbackDuringAReconnect(t *testing.T) {
	gate := make(chan struct{})
	h := newRefreshHarness(t, func(n int32) fetchStep {
		if n == 2 {
			return fetchStep{gate: gate}
		}
		return fetchStep{}
	})
	var (
		handle   atomic.Pointer[mosub.Subscription]
		armed    atomic.Bool
		once     sync.Once
		returned = make(chan time.Duration, 1)
	)
	sub, err := h.svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func([]models.MarketOverviewEntry) {
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
	first := h.nextConn(t)

	manual := make(chan error, 1)
	go func() { manual <- sub.RefreshSnapshot(context.Background()) }()
	h.awaitFetch(t, 2)
	close(first.drop)
	h.nextConn(t)
	// Let the reconnect's refresh start and wait for the refresh lock.
	time.Sleep(200 * time.Millisecond)
	armed.Store(true)
	close(gate)

	select {
	case took := <-returned:
		if took > promptClose {
			t.Fatalf("Close from the manual refresh callback took %v", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close from the manual refresh callback blocked")
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

// POLY-9018 parity: seeded schedules of manual refreshes, reconnect
// refreshes, socket drops, live publications, failing and slow snapshot
// fetches, and closes from OnEvent, from OnError and from another goroutine.
//
// Randomized per seed: each fetch's delay (up to 400ms, so refreshes overlap
// a reconnect's 250-500ms backoff) and whether it fails; a sequence of up
// to eight actions (drop the socket, start a manual refresh, send one to
// three publications, pause); and how the subscription is closed: from the
// k-th OnEvent after the schedule starts, from the first OnError, or from
// another goroutine after a delay. Invariants: a Close from a callback
// returns at once; every Close, manual refresh and the worker finish; and no
// callback starts once every Close and manual refresh has returned. Replay
// one schedule with POLYESTER_INTERLEAVING_SEED=<seed>; run more with
// POLYESTER_INTERLEAVING_SEEDS=<count>.
func TestOverviewRefreshAndCloseInterleavings(t *testing.T) {
	seeds := closeScheduleSeeds(t)
	if len(seeds) > 8 && testing.Short() {
		seeds = seeds[:8]
	}
	for _, seed := range seeds {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) { runRefreshSchedule(t, seed) })
	}
}

func runRefreshSchedule(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 1))
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
	h := newRefreshHarness(t, func(n int32) fetchStep {
		if int(n) < len(steps) {
			return steps[n]
		}
		return fetchStep{}
	})
	closeMode := []string{"event", "error", "outside"}[rng.IntN(3)]
	closeAtEvent := int32(1 + rng.IntN(3))
	outsideDelay := []time.Duration{0, time.Millisecond, 50 * time.Millisecond, 400 * time.Millisecond, 900 * time.Millisecond}[rng.IntN(5)]

	var (
		handle      atomic.Pointer[mosub.Subscription]
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
	sub, err := h.svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
		OnEvent: func([]models.MarketOverviewEntry) {
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

	price := int64(100)
	for range 1 + rng.IntN(8) {
		switch rng.IntN(4) {
		case 0: // drop the socket; the worker reconnects and refreshes
			if conn != nil {
				close(conn.drop)
				conn = nil
			}
		case 1: // a manual refresh, as a caller would make
			manualCalls.Add(1)
			go func() {
				defer manualCalls.Done()
				_ = sub.RefreshSnapshot(context.Background())
			}()
		case 2: // live publications on the current socket
			if conn == nil {
				select {
				case conn = <-h.conns:
				default:
				}
			}
			if conn != nil {
				for range 1 + rng.IntN(3) {
					price++
					select {
					case conn.frames <- overviewTicketFrame(price):
					default:
					}
				}
			}
		case 3:
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
