package realtime

import (
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
)

// A consumer that joins an order stream gets OrderStreamConnected first. Joining
// while the stream is disconnected, it waits for the reconnect and then gets
// Connected before the cached snapshot, ready and stale state, as a consumer
// joining a connected stream does.
//
// Each schedule draws, from one seed, the cached view (orders, ready, stale),
// the close code, when the consumer joins (before the new connection, before
// its subscription is confirmed, or once reconnected), the pauses around the
// join, and what the resumed subscription sends next. Set
// POLYESTER_INTERLEAVING_SEED to replay a failing run and
// POLYESTER_INTERLEAVINGS to run more schedules.
func TestPrivateOrdersConsumerJoiningDuringADisconnectionGetsConnectedFirst(t *testing.T) {
	base := uint64(0)
	if raw := os.Getenv("POLYESTER_INTERLEAVING_SEED"); raw != "" {
		seed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		base = seed
	} else {
		base = rand.Uint64()
	}
	count := 32
	if raw := os.Getenv("POLYESTER_INTERLEAVINGS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatal(err)
		}
		count = n
	}
	for offset := range uint64(count) {
		seed := base + offset
		t.Run(fmt.Sprint(seed), func(t *testing.T) { lateJoinerSchedule(t, seed) })
	}
}

// collectEvents reads events until none arrives for quiet.
func collectEvents(sub *Subscription[OrderStreamEvent], quiet time.Duration) []OrderStreamEvent {
	var events []OrderStreamEvent
	for {
		select {
		case event, ok := <-sub.Messages():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-time.After(quiet):
			return events
		}
	}
}

func lateJoinerSchedule(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, seed))
	cached := make([]*orderv1.Order, rng.IntN(4))
	wantIDs := make([]uint64, len(cached))
	for i := range cached {
		cached[i] = testOrder(uint64(i+1), 1)
		wantIDs[i] = uint64(i + 1)
	}
	ready := rng.IntN(2) == 0
	stale := rng.IntN(2) == 0
	closeCode := []int{4408, 1001, 4401}[rng.IntN(3)]
	join := []string{"before_connection", "before_confirm", "before_confirm", "reconnected"}[rng.IntN(4)]
	pause := time.Duration(rng.IntN(20_000)) * time.Microsecond
	after := []string{"nothing", "ready", "snapshot", "available"}[rng.IntN(4)]
	detail := fmt.Sprintf("seed=%d orders=%d ready=%v stale=%v close=%d join=%s after=%s: replay with POLYESTER_INTERLEAVING_SEED",
		seed, len(cached), ready, stale, closeCode, join, after)

	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	first, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", cached...)
	expectEvent[OrderSnapshot](t, first)
	if ready {
		seq++
		ws.send(t, "s1", seq, readyFrame())
		expectEvent[OrderReady](t, first)
	}
	if stale {
		seq++
		ws.send(t, "s1", seq, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Unavailable{
			Unavailable: &privatewsv1.Unavailable{Reason: privatewsv1.UnavailableReason_VIEW_BEHIND},
		}})
		expectEvent[OrderStale](t, first)
	}
	ws.closeWith(closeCode)
	expectEvent[OrderStreamDisconnected](t, first)

	var late *Subscription[OrderStreamEvent]
	joinNow := func() {
		time.Sleep(pause)
		late = subscribeOrders(t, c, testAccount)
	}
	if join == "before_connection" {
		joinNow()
	}
	second := f.accept(t)
	second.expectAuth(t)
	request := second.expectSubscribe(t)
	if join == "before_confirm" {
		joinNow()
		time.Sleep(time.Duration(rng.IntN(20_000)) * time.Microsecond)
	}
	second.confirm(t, "s2", request)
	expectEvent[OrderStreamConnected](t, first)
	if join == "reconnected" {
		joinNow()
	}
	switch after {
	case "ready":
		second.send(t, "s2", 1, readyFrame())
	case "snapshot":
		second.snapshot(t, "s2", 1, "c2", testOrder(9, 1))
	case "available":
		second.send(t, "s2", 1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Available{Available: &privatewsv1.Available{}}})
	}
	events := collectEvents(late, 100*time.Millisecond)
	kinds := make([]string, len(events))
	for i, event := range events {
		kinds[i] = fmt.Sprintf("%T", event)
	}

	if len(events) < 2 {
		t.Fatalf("events %v (%s)", kinds, detail)
	}
	if _, ok := events[0].(OrderStreamConnected); !ok {
		t.Fatalf("first event of %v; want OrderStreamConnected first (%s)", kinds, detail)
	}
	connected := 0
	for _, event := range events {
		if _, ok := event.(OrderStreamConnected); ok {
			connected++
		}
	}
	if connected != 1 {
		t.Fatalf("events %v (%s)", kinds, detail)
	}
	snapshot, ok := events[1].(OrderSnapshot)
	if !ok || !reflect.DeepEqual(orderIDs(snapshot.Orders), ids(wantIDs...)) {
		t.Fatalf("events %v; want the cached snapshot second (%s)", kinds, detail)
	}
	if after == "snapshot" {
		last, ok := events[len(events)-1].(OrderSnapshot)
		if !ok || !reflect.DeepEqual(orderIDs(last.Orders), ids(9)) {
			t.Fatalf("events %v; want the new snapshot last (%s)", kinds, detail)
		}
	}
}
