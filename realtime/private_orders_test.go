package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const testWait = 5 * time.Second

// fakePrivateServer serves the realtime token endpoints and /v1/ws/private.
type fakePrivateServer struct {
	srv           *httptest.Server
	conns         chan *fakeWS
	connectTokens atomic.Int64
	subTokens     atomic.Int64

	mu          sync.Mutex
	channels    []string
	wsPaths     []string
	subprotocol []string
	all         []*fakeWS
}

func newFakePrivateServer(t *testing.T) *fakePrivateServer {
	t.Helper()
	f := &fakePrivateServer{conns: make(chan *fakeWS, 64)}
	writeToken := func(w http.ResponseWriter, token string) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/rt/token", func(w http.ResponseWriter, r *http.Request) {
		writeToken(w, fmt.Sprintf("connect-%d", f.connectTokens.Add(1)))
	})
	mux.HandleFunc("/v1/rt/subscribe", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.channels = append(f.channels, r.URL.Query().Get("channel"))
		f.mu.Unlock()
		writeToken(w, fmt.Sprintf("sub-%d", f.subTokens.Add(1)))
	})
	upgrader := websocket.Upgrader{Subprotocols: []string{privateWSSubprotocol}}
	mux.HandleFunc("/v1/ws/private", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.wsPaths = append(f.wsPaths, r.URL.Path)
		f.subprotocol = append(f.subprotocol, r.Header.Get("Sec-Websocket-Protocol"))
		f.mu.Unlock()
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		fw := newFakeWS(ws)
		f.mu.Lock()
		f.all = append(f.all, fw)
		f.mu.Unlock()
		f.conns <- fw
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(func() {
		f.mu.Lock()
		all := f.all
		f.mu.Unlock()
		for _, fw := range all {
			_ = fw.ws.Close()
		}
		f.srv.Close()
	})
	return f
}

func (f *fakePrivateServer) accept(t *testing.T) *fakeWS {
	t.Helper()
	select {
	case fw := <-f.conns:
		return fw
	case <-time.After(testWait):
		t.Fatal("no private WebSocket connection")
		return nil
	}
}

func (f *fakePrivateServer) expectNoConnection(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case <-f.conns:
		t.Fatal("unexpected private WebSocket connection")
	case <-time.After(wait):
	}
}

func (f *fakePrivateServer) subscribeChannels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.channels...)
}

// fakeWS is the server side of one private WebSocket.
type fakeWS struct {
	ws        *websocket.Conn
	in        chan *privatewsv1.ClientMessage
	done      chan struct{}
	closeCode atomic.Int64
}

func newFakeWS(ws *websocket.Conn) *fakeWS {
	fw := &fakeWS{ws: ws, in: make(chan *privatewsv1.ClientMessage, 64), done: make(chan struct{})}
	go func() {
		defer close(fw.done)
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) {
					fw.closeCode.Store(int64(closeErr.Code))
				}
				return
			}
			msg := &privatewsv1.ClientMessage{}
			if proto.Unmarshal(data, msg) == nil {
				fw.in <- msg
			}
		}
	}()
	return fw
}

func (fw *fakeWS) next(t *testing.T) *privatewsv1.ClientMessage {
	t.Helper()
	select {
	case msg := <-fw.in:
		return msg
	case <-time.After(testWait):
		t.Fatal("no client frame")
		return nil
	}
}

func (fw *fakeWS) expectNone(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case msg := <-fw.in:
		t.Fatalf("unexpected client frame %v", msg)
	case <-time.After(wait):
	}
}

func (fw *fakeWS) expectAuth(t *testing.T) string {
	t.Helper()
	msg := fw.next(t)
	if msg.GetAuth() == nil {
		t.Fatalf("want auth, got %v", msg)
	}
	return msg.GetAuth().GetToken()
}

func (fw *fakeWS) expectSubscribe(t *testing.T) *privatewsv1.Subscribe {
	t.Helper()
	msg := fw.next(t)
	if msg.GetSubscribe() == nil {
		t.Fatalf("want subscribe, got %v", msg)
	}
	return msg.GetSubscribe()
}

func (fw *fakeWS) expectUnsubscribe(t *testing.T) *privatewsv1.Unsubscribe {
	t.Helper()
	msg := fw.next(t)
	if msg.GetUnsubscribe() == nil {
		t.Fatalf("want unsubscribe, got %v", msg)
	}
	return msg.GetUnsubscribe()
}

func (fw *fakeWS) sendRaw(t *testing.T, data []byte) {
	t.Helper()
	if err := fw.ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("server write: %v", err)
	}
}

func (fw *fakeWS) send(t *testing.T, sub string, seq uint64, msg *privatewsv1.ServerMessage) {
	t.Helper()
	msg.Sub = sub
	msg.Seq = seq
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	fw.sendRaw(t, data)
}

func (fw *fakeWS) closeWith(code int) {
	_ = fw.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""), time.Now().Add(time.Second))
	_ = fw.ws.Close()
}

func (fw *fakeWS) waitClosed(t *testing.T) int {
	t.Helper()
	select {
	case <-fw.done:
		return int(fw.closeCode.Load())
	case <-time.After(testWait):
		t.Fatal("client did not close the connection")
		return 0
	}
}

func (fw *fakeWS) confirm(t *testing.T, sub string, request *privatewsv1.Subscribe) {
	t.Helper()
	fw.send(t, sub, 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Subscribed{
		Subscribed: &privatewsv1.Subscribed{Id: request.GetId(), Account: request.GetAccount()},
	}})
}

func (fw *fakeWS) snapshot(t *testing.T, sub string, firstSeq uint64, cursor string, orders ...*orderv1.Order) uint64 {
	t.Helper()
	fw.send(t, sub, firstSeq, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotBegin{SnapshotBegin: &privatewsv1.SnapshotBegin{Epoch: 1}}})
	fw.send(t, sub, firstSeq+1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotPage{SnapshotPage: &privatewsv1.SnapshotPage{Orders: orders}}})
	fw.send(t, sub, firstSeq+2, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotEnd{
		SnapshotEnd: &privatewsv1.SnapshotEnd{Cursor: cursor, Count: uint32(len(orders))},
	}})
	return firstSeq + 2
}

func unitFrame(next string, orders []*orderv1.Order, removed []uint64, events ...*privatewsv1.UnitEvent) *privatewsv1.ServerMessage {
	return &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Unit{Unit: &privatewsv1.Unit{
		NextCursor: next, Orders: orders, RemovedOrderIds: removed, Events: events,
	}}}
}

func testOrder(id uint64, version uint32) *orderv1.Order {
	return &orderv1.Order{OrderId: id, SymbolId: 1, Version: version, Status: orderv1.OrderStatus_WORKING, OrigQtyScaled: 10, LeavesQtyScaled: 10, PriceTicks: 1_000_000_000}
}

const testAccount uint64 = 42

func testTiming() privateStreamTiming {
	return privateStreamTiming{
		heartbeatTimeout: 10 * time.Second,
		backoffBase:      5 * time.Millisecond,
		backoffMax:       20 * time.Millisecond,
		stableAfter:      time.Hour,
		writeTimeout:     time.Second,
		dialTimeout:      2 * time.Second,
	}
}

func newTestOrderClient(t *testing.T, f *fakePrivateServer, timing privateStreamTiming) *Client {
	t.Helper()
	c := NewClient("ws://127.0.0.1:1", f.srv.URL, testCreds(t), nil, 64)
	c.privateTiming = timing
	t.Cleanup(c.Close)
	return c
}

func subscribeOrders(t *testing.T, c *Client, account uint64) *Subscription[OrderStreamEvent] {
	t.Helper()
	sub, err := c.SubscribeOrders(context.Background(), codecs.FormatID(account))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub
}

func nextEvent(t *testing.T, sub *Subscription[OrderStreamEvent]) OrderStreamEvent {
	t.Helper()
	select {
	case event, ok := <-sub.Messages():
		if !ok {
			t.Fatalf("order stream ended: %v", sub.Err())
		}
		return event
	case <-time.After(testWait):
		t.Fatal("no order stream event")
		return nil
	}
}

func expectEvent[T OrderStreamEvent](t *testing.T, sub *Subscription[OrderStreamEvent]) T {
	t.Helper()
	event := nextEvent(t, sub)
	typed, ok := event.(T)
	if !ok {
		var want T
		t.Fatalf("want %T, got %#v", want, event)
	}
	return typed
}

func expectNoEvent(t *testing.T, sub *Subscription[OrderStreamEvent], wait time.Duration) {
	t.Helper()
	select {
	case event, ok := <-sub.Messages():
		t.Fatalf("unexpected event %#v (open=%v)", event, ok)
	case <-time.After(wait):
	}
}

func expectEnded(t *testing.T, sub *Subscription[OrderStreamEvent]) {
	t.Helper()
	select {
	case event, ok := <-sub.Messages():
		if ok {
			t.Fatalf("want the stream to end, got %#v", event)
		}
	case <-time.After(testWait):
		t.Fatal("order stream did not end")
	}
	<-sub.Done()
}

// connectOrders subscribes, answers auth and subscribe, and confirms the
// subscription as server subscription "s1".
func connectOrders(t *testing.T, f *fakePrivateServer, c *Client) (*Subscription[OrderStreamEvent], *fakeWS) {
	t.Helper()
	sub := subscribeOrders(t, c, testAccount)
	ws := f.accept(t)
	ws.expectAuth(t)
	ws.confirm(t, "s1", ws.expectSubscribe(t))
	expectEvent[OrderStreamConnected](t, sub)
	return sub, ws
}

func orderIDs(orders []models.Order) []string {
	out := make([]string, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.OrderID)
	}
	return out
}

func ids(values ...uint64) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, codecs.FormatID(v))
	}
	return out
}

func noPrivateStreamGoroutines() (bool, string) {
	buf := make([]byte, 1<<20)
	stacks := string(buf[:runtime.Stack(buf, true)])
	for _, g := range strings.Split(stacks, "\n\n") {
		if strings.Contains(g, "(*privateOrderStream)") {
			return false, g
		}
	}
	return true, ""
}

func waitNoPrivateStreamGoroutines(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for {
		ok, stack := noPrivateStreamGoroutines()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("private order stream goroutine still running:\n%s", stack)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPrivateWSURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.example.test":       "wss://api.example.test/v1/ws/private",
		"https://api.example.test/":      "wss://api.example.test/v1/ws/private",
		"http://127.0.0.1:8080/prefix/":  "ws://127.0.0.1:8080/prefix/v1/ws/private",
		"https://api.example.test/base?": "wss://api.example.test/base/v1/ws/private",
	} {
		got, err := privateWSURL(in)
		if err != nil || got != want {
			t.Fatalf("privateWSURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := privateWSURL("ftp://api.example.test"); err == nil {
		t.Fatal("want an error for a non-HTTP URL")
	}
}

func TestPrivateOrdersAuthFirstThenBaseChannelSubscribe(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub := subscribeOrders(t, c, testAccount)
	ws := f.accept(t)

	if token := ws.expectAuth(t); token != "connect-1" {
		t.Fatalf("auth token %q", token)
	}
	request := ws.expectSubscribe(t)
	if request.GetChannel() != privatewsv1.Channel_ORDERS || request.GetAccount() != testAccount ||
		request.GetToken() != "sub-1" || request.GetCursor() != "" || request.GetId() == "" {
		t.Fatalf("subscribe %v", request)
	}
	wantChannel := "private:spot:orders:" + codecs.FormatID(testAccount)
	if got := f.subscribeChannels(); !reflect.DeepEqual(got, []string{wantChannel}) {
		t.Fatalf("subscribe token channels %v, want %v", got, []string{wantChannel})
	}
	f.mu.Lock()
	paths, protocols := f.wsPaths, f.subprotocol
	f.mu.Unlock()
	if !reflect.DeepEqual(paths, []string{"/v1/ws/private"}) || !reflect.DeepEqual(protocols, []string{privateWSSubprotocol}) {
		t.Fatalf("socket paths %v, subprotocols %v", paths, protocols)
	}
	ws.confirm(t, "s1", request)
	expectEvent[OrderStreamConnected](t, sub)
}

func TestPrivateOrdersSnapshotInstalledAtSnapshotEnd(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	first, second := testOrder(1, 3), testOrder(2, 1)
	ws.send(t, "s1", 1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotBegin{SnapshotBegin: &privatewsv1.SnapshotBegin{Epoch: 7}}})
	ws.send(t, "s1", 2, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotPage{SnapshotPage: &privatewsv1.SnapshotPage{Orders: []*orderv1.Order{first}}}})
	ws.send(t, "s1", 3, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotPage{SnapshotPage: &privatewsv1.SnapshotPage{Orders: []*orderv1.Order{second}}}})
	expectNoEvent(t, sub, 50*time.Millisecond)
	ws.send(t, "s1", 4, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotEnd{SnapshotEnd: &privatewsv1.SnapshotEnd{Cursor: "c1", Count: 2}}})

	snapshot := expectEvent[OrderSnapshot](t, sub)
	want := []models.Order{decode.OrderFromProto(first), decode.OrderFromProto(second)}
	if !reflect.DeepEqual(snapshot.Orders, want) {
		t.Fatalf("snapshot %#v, want %#v", snapshot.Orders, want)
	}
	if snapshot.Orders[0].Status != "working" || snapshot.Orders[0].Version != 3 {
		t.Fatalf("decoded order %#v", snapshot.Orders[0])
	}
}

func TestPrivateOrdersSnapshotCountMismatchTakesFreshSnapshot(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	ws.send(t, "s1", 1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotBegin{SnapshotBegin: &privatewsv1.SnapshotBegin{}}})
	ws.send(t, "s1", 2, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotPage{SnapshotPage: &privatewsv1.SnapshotPage{Orders: []*orderv1.Order{testOrder(1, 1)}}}})
	ws.send(t, "s1", 3, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SnapshotEnd{SnapshotEnd: &privatewsv1.SnapshotEnd{Cursor: "c1", Count: 2}}})

	if got := ws.expectUnsubscribe(t).GetSub(); got != "s1" {
		t.Fatalf("unsubscribed %q", got)
	}
	if cursor := ws.expectSubscribe(t).GetCursor(); cursor != "" {
		t.Fatalf("resubscribe cursor %q, want a fresh snapshot", cursor)
	}
	expectEvent[OrderStreamDisconnected](t, sub)
	expectNoEvent(t, sub, 50*time.Millisecond)
}

func TestPrivateOrdersUnitDecodesOrdersRemovalsAndEvents(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1), testOrder(2, 1))
	expectEvent[OrderSnapshot](t, sub)

	filled := testOrder(1, 2)
	filled.Status = orderv1.OrderStatus_FILLED
	filled.CumQtyScaled = 10
	ws.send(t, "s1", seq+1, unitFrame("c2", []*orderv1.Order{filled}, []uint64{2},
		&privatewsv1.UnitEvent{Kind: &privatewsv1.UnitEvent_Order{Order: &privatewsv1.OrderEvent{
			OrderId: 1, Version: 2, Kind: privatewsv1.OrderEvent_FILLED,
			Fill: &privatewsv1.Fill{MatchId: 99, Ordinal: 1, Role: privatewsv1.Fill_MAKER, QtyScaled: 10, PriceTicks: 1_000_000_000},
		}}},
		&privatewsv1.UnitEvent{Kind: &privatewsv1.UnitEvent_Outcome{Outcome: &privatewsv1.CommandOutcome{
			Kind: privatewsv1.CommandOutcome_CANCEL_REJECTED, OrderId: 2, ClientOrderId: "cl-2",
			Reason: orderv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS,
		}}},
	))

	unit := expectEvent[OrderUnit](t, sub)
	if !reflect.DeepEqual(unit.Orders, []models.Order{decode.OrderFromProto(filled)}) {
		t.Fatalf("unit orders %#v", unit.Orders)
	}
	if !reflect.DeepEqual(unit.RemovedOrderIDs, ids(2)) {
		t.Fatalf("removed %v", unit.RemovedOrderIDs)
	}
	if len(unit.Events) != 2 || unit.Events[0].Order == nil || unit.Events[1].Outcome == nil {
		t.Fatalf("events %#v", unit.Events)
	}
	event := unit.Events[0].Order
	if event.OrderID != codecs.FormatID(1) || event.Kind != models.OrderEventFilled || event.Version != 2 ||
		event.Fill == nil || event.Fill.MatchID != "99" || event.Fill.Role != "maker" ||
		event.Fill.Qty.Scaled() != 10 || event.Fill.Price.Ticks() != 1_000_000_000 {
		t.Fatalf("order event %#v fill %#v", event, event.Fill)
	}
	if symbolID := event.Fill.Qty.SymbolID(); symbolID == nil || *symbolID != 1 {
		t.Fatalf("fill quantity symbol %v", symbolID)
	}
	outcome := unit.Events[1].Outcome
	if outcome.Kind != models.OrderOutcomeCancelRejected || outcome.OrderID != codecs.FormatID(2) ||
		outcome.ClientOrderID != "cl-2" || outcome.Reason != "INSUFFICIENT_FUNDS" || outcome.NewOrderID != "" {
		t.Fatalf("outcome %#v", outcome)
	}

	// A consumer joining now starts from the applied state.
	late := subscribeOrders(t, c, testAccount)
	expectEvent[OrderStreamConnected](t, late)
	if got := expectEvent[OrderSnapshot](t, late); !reflect.DeepEqual(got.Orders, []models.Order{decode.OrderFromProto(filled)}) {
		t.Fatalf("late snapshot %#v", got.Orders)
	}
}

func TestPrivateOrdersSeqGapResubscribesFromCursor(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	ws.send(t, "s1", seq+2, unitFrame("c9", []*orderv1.Order{testOrder(1, 2)}, nil))
	if got := ws.expectUnsubscribe(t).GetSub(); got != "s1" {
		t.Fatalf("unsubscribed %q", got)
	}
	request := ws.expectSubscribe(t)
	if request.GetCursor() != "c1" {
		t.Fatalf("resubscribe cursor %q, want c1", request.GetCursor())
	}
	expectEvent[OrderStreamDisconnected](t, sub)

	// Frames of the dropped subscription are ignored.
	ws.send(t, "s1", seq+3, unitFrame("c10", []*orderv1.Order{testOrder(1, 3)}, nil))
	ws.confirm(t, "s2", request)
	expectEvent[OrderStreamConnected](t, sub)
	ws.send(t, "s2", 1, unitFrame("c2", []*orderv1.Order{testOrder(1, 2)}, nil))
	if unit := expectEvent[OrderUnit](t, sub); !reflect.DeepEqual(orderIDs(unit.Orders), ids(1)) || unit.Orders[0].Version != 2 {
		t.Fatalf("unit after resume %#v", unit)
	}
}

func TestPrivateOrdersUnknownFrameKindCountsInSeq(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1")
	expectEvent[OrderSnapshot](t, sub)

	// A frame kind from a newer protocol: field 999 is no frame this SDK knows.
	unknown, err := proto.Marshal(&privatewsv1.ServerMessage{Sub: "s1", Seq: seq + 1})
	if err != nil {
		t.Fatal(err)
	}
	unknown = protowire.AppendTag(unknown, 999, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, []byte{0x08, 0x01})
	ws.sendRaw(t, unknown)
	ws.send(t, "s1", seq+2, unitFrame("c2", []*orderv1.Order{testOrder(1, 1)}, nil))

	if unit := expectEvent[OrderUnit](t, sub); !reflect.DeepEqual(orderIDs(unit.Orders), ids(1)) {
		t.Fatalf("unit %#v", unit)
	}
	ws.expectNone(t, 50*time.Millisecond)
}

func TestPrivateOrdersResyncReplacesStateWithNextSnapshot(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1), testOrder(2, 1))
	expectEvent[OrderSnapshot](t, sub)

	ws.send(t, "s1", seq+1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_ResyncRequired{
		ResyncRequired: &privatewsv1.ResyncRequired{Reason: privatewsv1.ResyncRequired_CURSOR_EXPIRED},
	}})
	ws.snapshot(t, "s1", seq+2, "c5", testOrder(3, 1))
	if snapshot := expectEvent[OrderSnapshot](t, sub); !reflect.DeepEqual(orderIDs(snapshot.Orders), ids(3)) {
		t.Fatalf("resync snapshot %v", orderIDs(snapshot.Orders))
	}

	// The replaced state is what a joining consumer gets; the server subscription is shared.
	late := subscribeOrders(t, c, testAccount)
	expectEvent[OrderStreamConnected](t, late)
	if snapshot := expectEvent[OrderSnapshot](t, late); !reflect.DeepEqual(orderIDs(snapshot.Orders), ids(3)) {
		t.Fatalf("late snapshot %v", orderIDs(snapshot.Orders))
	}
	ws.expectNone(t, 50*time.Millisecond)
}

func TestPrivateOrdersUnavailableAndAvailableSetStale(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	ws.send(t, "s1", 1, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Unavailable{
		Unavailable: &privatewsv1.Unavailable{Reason: privatewsv1.UnavailableReason_VIEW_BEHIND},
	}})
	if stale := expectEvent[OrderStale](t, sub); !stale.Stale || stale.Reason != "view_behind" {
		t.Fatalf("stale %#v", stale)
	}
	ws.send(t, "s1", 2, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Available{Available: &privatewsv1.Available{}}})
	if stale := expectEvent[OrderStale](t, sub); stale.Stale {
		t.Fatalf("stale %#v", stale)
	}
}

func TestPrivateOrdersSubscribeExpiringRefreshesSubscribeToken(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	ws.send(t, "s1", 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SubscribeExpiring{
		SubscribeExpiring: &privatewsv1.SubscribeExpiring{ExpiresTsSec: 1},
	}})
	refresh := ws.next(t).GetSubscribeRefresh()
	if refresh == nil || refresh.GetSub() != "s1" || refresh.GetToken() != "sub-2" || refresh.GetId() == "" {
		t.Fatalf("refresh %v", refresh)
	}
	ws.send(t, "s1", 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_SubscribeRefreshed{
		SubscribeRefreshed: &privatewsv1.SubscribeRefreshed{Id: refresh.GetId(), ExpiresTsSec: 2},
	}})
	expectNoEvent(t, sub, 50*time.Millisecond)
	ws.expectNone(t, 50*time.Millisecond)
}

func TestPrivateOrdersTokenExpiringReauthenticates(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	ws.send(t, "", 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_TokenExpiring{
		TokenExpiring: &privatewsv1.TokenExpiring{ExpiresTsSec: 1},
	}})
	if token := ws.expectAuth(t); token != "connect-2" {
		t.Fatalf("re-auth token %q", token)
	}
	expectNoEvent(t, sub, 50*time.Millisecond)
}

func TestPrivateOrdersHeartbeatTimeoutReconnectsWithCursor(t *testing.T) {
	f := newFakePrivateServer(t)
	timing := testTiming()
	timing.heartbeatTimeout = 400 * time.Millisecond
	c := newTestOrderClient(t, f, timing)
	sub, ws := connectOrders(t, f, c)
	ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	// Heartbeats keep the connection.
	for range 3 {
		time.Sleep(200 * time.Millisecond)
		ws.send(t, "", 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Heartbeat{Heartbeat: &privatewsv1.Heartbeat{ServerTsNs: 1}}})
	}
	expectNoEvent(t, sub, 0)

	// Silence drops it.
	if code := ws.waitClosed(t); code != closeHeartbeatTimeout {
		t.Fatalf("close code %d, want %d", code, closeHeartbeatTimeout)
	}
	expectEvent[OrderStreamDisconnected](t, sub)
	second := f.accept(t)
	if token := second.expectAuth(t); token != "connect-2" {
		t.Fatalf("reconnect auth token %q", token)
	}
	request := second.expectSubscribe(t)
	if request.GetCursor() != "c1" {
		t.Fatalf("reconnect cursor %q, want c1", request.GetCursor())
	}
	second.confirm(t, "s2", request)
	expectEvent[OrderStreamConnected](t, sub)
}

func TestPrivateOrdersClose4403IsTerminal(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)

	ws.closeWith(4403)
	streamErr := expectEvent[OrderStreamError](t, sub)
	var realtimeErr *sdkerrors.RealtimeError
	if !errors.As(streamErr.Err, &realtimeErr) || realtimeErr.CloseCode != 4403 {
		t.Fatalf("error %#v", streamErr.Err)
	}
	expectEvent[OrderStreamDisconnected](t, sub)
	expectEnded(t, sub)
	if !errors.As(sub.Err(), &realtimeErr) || realtimeErr.CloseCode != 4403 {
		t.Fatalf("Err() = %v", sub.Err())
	}
	f.expectNoConnection(t, 100*time.Millisecond)
	if n := f.connectTokens.Load(); n != 1 {
		t.Fatalf("connect tokens fetched %d, want 1", n)
	}
	waitNoPrivateStreamGoroutines(t)
}

func TestPrivateOrdersClose4408Reconnects(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	ws.closeWith(4408)
	expectEvent[OrderStreamDisconnected](t, sub)
	second := f.accept(t)
	second.expectAuth(t)
	request := second.expectSubscribe(t)
	if request.GetCursor() != "c1" {
		t.Fatalf("reconnect cursor %q, want c1", request.GetCursor())
	}
	second.confirm(t, "s7", request)
	expectEvent[OrderStreamConnected](t, sub)
	second.send(t, "s7", 1, unitFrame("c2", []*orderv1.Order{testOrder(1, 2)}, nil))
	expectEvent[OrderUnit](t, sub)
}

func TestPrivateOrdersForbiddenSubscribeSurfacesAndEnds(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub := subscribeOrders(t, c, testAccount)
	ws := f.accept(t)
	ws.expectAuth(t)
	request := ws.expectSubscribe(t)

	ws.send(t, "", 0, &privatewsv1.ServerMessage{Frame: &privatewsv1.ServerMessage_Error{Error: &privatewsv1.RequestError{
		Id: request.GetId(), Code: privatewsv1.RequestError_FORBIDDEN, Message: "not your account",
	}}})
	streamErr := expectEvent[OrderStreamError](t, sub)
	var realtimeErr *sdkerrors.RealtimeError
	if !errors.As(streamErr.Err, &realtimeErr) || realtimeErr.Code != "FORBIDDEN" || realtimeErr.Msg != "not your account" {
		t.Fatalf("error %#v", streamErr.Err)
	}
	expectEvent[OrderStreamDisconnected](t, sub)
	expectEnded(t, sub)
	if !errors.As(sub.Err(), &realtimeErr) || realtimeErr.Code != "FORBIDDEN" {
		t.Fatalf("Err() = %v", sub.Err())
	}
	// The last subscription gone, the idle connection closes.
	if code := ws.waitClosed(t); code != websocket.CloseNormalClosure {
		t.Fatalf("close code %d, want 1000", code)
	}
}

func TestPrivateOrdersSharesOneServerSubscriptionPerAccount(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	first, ws := connectOrders(t, f, c)
	second := subscribeOrders(t, c, testAccount)
	expectEvent[OrderStreamConnected](t, second)
	ws.expectNone(t, 50*time.Millisecond)

	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, first)
	expectEvent[OrderSnapshot](t, second)

	// Another account rides the same connection.
	other := subscribeOrders(t, c, testAccount+1)
	request := ws.expectSubscribe(t)
	if request.GetAccount() != testAccount+1 {
		t.Fatalf("second account subscribe %v", request)
	}
	ws.confirm(t, "s2", request)
	expectEvent[OrderStreamConnected](t, other)
	f.expectNoConnection(t, 50*time.Millisecond)

	first.Close()
	ws.expectNone(t, 50*time.Millisecond)
	ws.send(t, "s1", seq+1, unitFrame("c2", []*orderv1.Order{testOrder(1, 2)}, nil))
	expectEvent[OrderUnit](t, second)

	second.Close()
	if got := ws.expectUnsubscribe(t).GetSub(); got != "s1" {
		t.Fatalf("unsubscribed %q", got)
	}
	other.Close()
	if got := ws.expectUnsubscribe(t).GetSub(); got != "s2" {
		t.Fatalf("unsubscribed %q", got)
	}
	if code := ws.waitClosed(t); code != websocket.CloseNormalClosure {
		t.Fatalf("close code %d, want 1000", code)
	}
}

func TestPrivateOrdersCloseStopsGoroutines(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	sub, ws := connectOrders(t, f, c)
	ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	expectEvent[OrderSnapshot](t, sub)

	sub.Close()
	if ok, stack := noPrivateStreamGoroutines(); !ok {
		t.Fatalf("goroutine still running after Close:\n%s", stack)
	}
	if got := ws.expectUnsubscribe(t).GetSub(); got != "s1" {
		t.Fatalf("unsubscribed %q", got)
	}
	if code := ws.waitClosed(t); code != websocket.CloseNormalClosure {
		t.Fatalf("close code %d, want 1000", code)
	}
	if _, ok := <-sub.Messages(); ok {
		t.Fatal("events channel still open after Close")
	}
	if sub.Err() != nil {
		t.Fatalf("Err() after Close = %v", sub.Err())
	}
}

func TestPrivateOrdersContextCancelStopsGoroutines(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := c.SubscribeOrders(ctx, codecs.FormatID(testAccount))
	if err != nil {
		t.Fatal(err)
	}
	ws := f.accept(t)
	ws.expectAuth(t)
	ws.confirm(t, "s1", ws.expectSubscribe(t))
	expectEvent[OrderStreamConnected](t, sub)

	cancel()
	select {
	case <-sub.Done():
	case <-time.After(testWait):
		t.Fatal("subscription did not stop on cancel")
	}
	waitNoPrivateStreamGoroutines(t)
	if code := ws.waitClosed(t); code != websocket.CloseNormalClosure {
		t.Fatalf("close code %d, want 1000", code)
	}
}

func TestPrivateOrdersCloseWhileConnecting(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	for range 20 {
		sub := subscribeOrders(t, c, testAccount)
		sub.Close()
		if ok, stack := noPrivateStreamGoroutines(); !ok {
			t.Fatalf("goroutine still running after Close:\n%s", stack)
		}
	}
}

func TestPrivateOrdersSlowConsumerFailsWithQueueOverflow(t *testing.T) {
	f := newFakePrivateServer(t)
	c := NewClient("ws://127.0.0.1:1", f.srv.URL, testCreds(t), nil, 2)
	c.privateTiming = testTiming()
	t.Cleanup(c.Close)
	sub := subscribeOrders(t, c, testAccount)
	ws := f.accept(t)
	ws.expectAuth(t)
	ws.confirm(t, "s1", ws.expectSubscribe(t))
	for seq := uint64(1); seq <= 3; seq++ {
		ws.send(t, "s1", seq, unitFrame(fmt.Sprint("c", seq), nil, nil))
	}
	if got := ws.expectUnsubscribe(t).GetSub(); got != "s1" {
		t.Fatalf("unsubscribed %q", got)
	}
	<-sub.Done()
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(sub.Err(), &overflow) {
		t.Fatalf("Err() = %v", sub.Err())
	}
}

func TestSubscribeOrdersValidatesInput(t *testing.T) {
	if _, err := NewClient("ws://127.0.0.1:1", "http://127.0.0.1:1", nil, nil, 1).SubscribeOrders(context.Background(), "abc"); err == nil {
		t.Fatal("want an auth error without credentials")
	} else {
		var authErr *sdkerrors.AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("want AuthError, got %T", err)
		}
	}
	c := NewClient("ws://127.0.0.1:1", "http://127.0.0.1:1", testCreds(t), nil, 1)
	defer c.Close()
	for _, account := range []string{"", "0OIl", "1"} {
		if _, err := c.SubscribeOrders(context.Background(), account); err == nil {
			t.Fatalf("account %q: want a validation error", account)
		}
	}
}
