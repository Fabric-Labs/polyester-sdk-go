package hardening_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1/orderbookv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// The server in this file behaves like the public stream contract: it accepts a
// subscribe at any depth, but publishes only on the tiers 1, 20, 50, 200 and 500,
// each as a SET diff of that tier's own top-N view. Snapshots answer at any depth
// the snapshot enum names, up to 1000.

const (
	tierUnit    = int64(1_000_000_000) // One quote unit, in price ticks.
	tierBestBid = 2000 * tierUnit
	tierBestAsk = 2001 * tierUnit
	tierWait    = 5 * time.Second
)

var publishedTiers = []int{1, 20, 50, 200, 500}

func tierChannel(depth int) string {
	return fmt.Sprintf("public:spot:orderbook:deltas:depth:%d:1:proto", depth)
}

func tierFor(depth int) int {
	for _, tier := range publishedTiers {
		if tier >= depth {
			return tier
		}
	}
	return 0
}

type tierConn struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (c *tierConn) write(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, frame)
}

type tierView struct {
	bids map[int64]int64
	asks map[int64]int64
	seq  uint64
}

// tierMarket is a book published per tier as SET diffs and read through the snapshot API.
type tierMarket struct {
	orderbookv1connect.UnimplementedOrderbookServiceHandler

	mu          sync.Mutex
	bids        map[int64]int64
	asks        map[int64]int64
	seq         uint64
	unavailable bool
	snapshots   []string
	delivered   map[string]int
	conns       map[string][]*tierConn
	published   map[int]tierView

	connections atomic.Int64
	subscribed  chan string
}

func newTierMarket(levels int) *tierMarket {
	m := &tierMarket{
		bids:       map[int64]int64{},
		asks:       map[int64]int64{},
		seq:        100,
		delivered:  map[string]int{},
		conns:      map[string][]*tierConn{},
		published:  map[int]tierView{},
		subscribed: make(chan string, 1024),
	}
	for i := range int64(levels) {
		m.bids[tierBestBid-i*tierUnit] = 5
		m.asks[tierBestAsk+i*tierUnit] = 5
	}
	for _, tier := range publishedTiers {
		bids, asks := m.viewLocked(tier)
		m.published[tier] = tierView{bids: bids, asks: asks, seq: m.seq}
	}
	return m
}

func topLevels(side map[int64]int64, depth int, descending bool) map[int64]int64 {
	prices := slices.Sorted(maps.Keys(side))
	if descending {
		slices.Reverse(prices)
	}
	out := make(map[int64]int64, depth)
	for _, price := range prices[:min(depth, len(prices))] {
		out[price] = side[price]
	}
	return out
}

func (m *tierMarket) viewLocked(depth int) (map[int64]int64, map[int64]int64) {
	return topLevels(m.bids, depth, true), topLevels(m.asks, depth, false)
}

func depthLevels(depth orderbookv1.Depth) int {
	var levels int
	if _, err := fmt.Sscanf(depth.String(), "DEPTH_%d", &levels); err != nil {
		return 0
	}
	return levels
}

func sortedProtoLevels(side map[int64]int64, descending bool) []*orderbookv1.PriceLevel {
	prices := slices.Sorted(maps.Keys(side))
	if descending {
		slices.Reverse(prices)
	}
	out := make([]*orderbookv1.PriceLevel, 0, len(prices))
	for _, price := range prices {
		out = append(out, &orderbookv1.PriceLevel{PriceTicks: price, QtyScaled: side[price]})
	}
	return out
}

func (m *tierMarket) GetOrderBook(_ context.Context, req *connect.Request[orderbookv1.GetOrderBookRequest]) (*connect.Response[orderbookv1.GetOrderBookResponse], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots = append(m.snapshots, req.Msg.GetDepth().String())
	response := &orderbookv1.GetOrderBookResponse{SymbolId: 1, BookSeq: m.seq, Available: !m.unavailable}
	if !m.unavailable {
		bids, asks := m.viewLocked(depthLevels(req.Msg.GetDepth()))
		response.Bids = sortedProtoLevels(bids, true)
		response.Asks = sortedProtoLevels(asks, false)
	}
	return connect.NewResponse(response), nil
}

func (m *tierMarket) serveWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	m.connections.Add(1)
	if _, _, err = conn.ReadMessage(); err != nil {
		return
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(1)); err != nil {
		return
	}
	_, command, err := conn.ReadMessage()
	if err != nil {
		return
	}
	start := strings.Index(string(command), "public:")
	if start < 1 {
		return
	}
	channel := string(command[start : start+int(command[start-1])])
	if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(2)); err != nil {
		return
	}
	entry := &tierConn{conn: conn}
	m.mu.Lock()
	m.conns[channel] = append(m.conns[channel], entry)
	m.mu.Unlock()
	m.subscribed <- channel
	for {
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
	}
}

type tierDelta struct {
	start, end uint64
	bids, asks map[int64]int64
	reset      bool
}

// send delivers one delta to everyone subscribed at depth, as given.
func (m *tierMarket) send(t *testing.T, depth int, delta tierDelta) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendLocked(t, depth, delta)
}

func (m *tierMarket) sendLocked(t *testing.T, depth int, delta tierDelta) {
	t.Helper()
	message := &orderbookv1.OrderBookDelta{
		SymbolId: 1, BookSeqStart: delta.start, BookSeqEnd: delta.end, Reset_: delta.reset,
		Bids: sortedProtoLevels(delta.bids, true), Asks: sortedProtoLevels(delta.asks, false),
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	channel := tierChannel(depth)
	for _, conn := range m.conns[channel] {
		if conn.write(hardening.CentrifugoPublication(payload)) == nil {
			m.delivered[channel]++
		}
	}
}

func levelDiff(previous, current map[int64]int64) map[int64]int64 {
	changes := map[int64]int64{}
	for price, qty := range current {
		if old, ok := previous[price]; !ok || old != qty {
			changes[price] = qty
		}
	}
	for price := range previous {
		if _, ok := current[price]; !ok {
			changes[price] = 0
		}
	}
	return changes
}

// publish advances the sequence and sends each tier's diff, on the tiers only.
func (m *tierMarket) publish(t *testing.T, change func(*tierMarket)) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	change(m)
	m.seq++
	for _, tier := range publishedTiers {
		bids, asks := m.viewLocked(tier)
		last := m.published[tier]
		bidDiff, askDiff := levelDiff(last.bids, bids), levelDiff(last.asks, asks)
		if len(bidDiff) == 0 && len(askDiff) == 0 {
			continue
		}
		m.sendLocked(t, tier, tierDelta{start: last.seq, end: m.seq, bids: bidDiff, asks: askDiff})
		m.published[tier] = tierView{bids: bids, asks: asks, seq: m.seq}
	}
}

func (m *tierMarket) update(change func(*tierMarket)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	change(m)
}

func (m *tierMarket) deliveredTo(depth int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.delivered[tierChannel(depth)]
}

func (m *tierMarket) snapshotRequests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.snapshots)
}

func (m *tierMarket) forgetConns() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.conns)
}

func (m *tierMarket) dropConns(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, conn := range m.conns[channel] {
		_ = conn.conn.Close()
	}
	delete(m.conns, channel)
}

func (m *tierMarket) nextSubscribed(t *testing.T) string {
	t.Helper()
	select {
	case channel := <-m.subscribed:
		return channel
	case <-time.After(tierWait):
		t.Fatal("no subscribe reached the server")
	}
	return ""
}

func newTierService(t *testing.T, levels int) (*services.OrderbookService, *tierMarket) {
	t.Helper()
	market := newTierMarket(levels)
	ws := hardening.NewTestServer(http.HandlerFunc(market.serveWS))
	t.Cleanup(ws.Close)
	mux := http.NewServeMux()
	path, handler := orderbookv1connect.NewOrderbookServiceHandler(market)
	mux.Handle(path, handler)
	api := hardening.NewTestServer(mux)
	t.Cleanup(api.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = api.URL
	rt := newRT("ws"+strings.TrimPrefix(ws.URL, "http"), api.URL, nil, tierWait)
	t.Cleanup(rt.Close)
	return services.NewOrderbookService(transport.NewFactory(cfg, nil, nil), cats, rt), market
}

func tierBook(t *testing.T, service *services.OrderbookService, opts services.CreateSubscriptionOptions) *orderbook.Subscription {
	t.Helper()
	opts.Symbol = "BTC-USDT"
	sub, err := service.CreateSubscription(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub
}

func nextTierView(t *testing.T, sub *orderbook.Subscription, what string) models.OrderbookData {
	t.Helper()
	select {
	case view, open := <-sub.Updates():
		if !open {
			t.Fatalf("no %s: stream ended: %v", what, sub.Err())
		}
		return view
	case <-time.After(tierWait):
		t.Fatalf("no %s within %s: the managed book went silent", what, tierWait)
	}
	return models.OrderbookData{}
}

func viewTicks(levels []models.OrderbookLevel) []int64 {
	out := make([]int64, 0, len(levels))
	for _, level := range levels {
		out = append(out, level.Price.Ticks())
	}
	return out
}

func viewQtys(levels []models.OrderbookLevel) []int64 {
	out := make([]int64, 0, len(levels))
	for _, level := range levels {
		out = append(out, level.Qty.Scaled())
	}
	return out
}

func ladder(from, step int64, count int) []int64 {
	out := make([]int64, 0, count)
	for i := range int64(count) {
		out = append(out, from+i*step)
	}
	return out
}

func assertSnapshots(t *testing.T, market *tierMarket, want ...string) {
	t.Helper()
	if got := market.snapshotRequests(); !slices.Equal(got, want) {
		t.Fatalf("snapshot requests = %v, want %v", got, want)
	}
}

// Without this, a server that publishes everywhere would make every test here pass.
func TestTierServerControlUnpublishedChannelIsAcceptedAndSilent(t *testing.T) {
	market := newTierMarket(600)
	ws := hardening.NewTestServer(http.HandlerFunc(market.serveWS))
	t.Cleanup(ws.Close)
	rt := newRT("ws"+strings.TrimPrefix(ws.URL, "http"), ws.URL, nil, tierWait)
	t.Cleanup(rt.Close)
	silent, err := realtime.SubscribeProto(context.Background(), rt, tierChannel(10), identityDecode)
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	if got := market.nextSubscribed(t); got != tierChannel(10) {
		t.Fatalf("subscribed %q", got)
	}
	live, err := realtime.SubscribeProto(context.Background(), rt, tierChannel(20), identityDecode)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if got := market.nextSubscribed(t); got != tierChannel(20) {
		t.Fatalf("subscribed %q", got)
	}
	market.publish(t, func(m *tierMarket) { m.bids[tierBestBid] = 7 })
	select {
	case <-live.Messages():
	case <-time.After(tierWait):
		t.Fatal("published tier delivered nothing")
	}
	select {
	case <-silent.Messages():
		t.Fatal("unpublished channel delivered a publication")
	case <-time.After(300 * time.Millisecond):
	}
	if silent.Err() != nil {
		t.Fatalf("the server accepted the unpublished subscribe, got %v", silent.Err())
	}
	if market.deliveredTo(10) != 0 || market.deliveredTo(20) != 1 {
		t.Fatalf("delivered = %v", market.delivered)
	}
}

func TestManagedBookAtDepthMeasuredSilentUpdatesOnDeltas(t *testing.T) {
	for _, depth := range []int{5, 10, 25, 100} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			service, market := newTierService(t, 600)
			sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: depth})
			first := nextTierView(t, sub, "first view")
			market.publish(t, func(m *tierMarket) { m.bids[tierBestBid] = 7 })
			updated := nextTierView(t, sub, "view after a market move")
			if market.deliveredTo(tierFor(depth)) != 1 {
				t.Fatal("no delta was delivered")
			}
			if first.Bids[0].Qty.Scaled() != 5 || updated.Bids[0].Qty.Scaled() != 7 {
				t.Fatalf("first %v updated %v", first.Bids[0], updated.Bids[0])
			}
			if updated.BookSeq != "101" || first.BookSeq != "100" {
				t.Fatalf("seq %s then %s", first.BookSeq, updated.BookSeq)
			}
			if len(updated.Bids) != depth || len(updated.Asks) != depth {
				t.Fatalf("view has %d bids and %d asks", len(updated.Bids), len(updated.Asks))
			}
		})
	}
}

func TestManagedBookEveryRequestedDepthRidesItsTierAndTrims(t *testing.T) {
	service, market := newTierService(t, 600)
	seen := map[int]bool{}
	qty := int64(5)
	for depth := 1; depth <= 500; depth++ {
		tier := tierFor(depth)
		seen[tier] = true
		sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: depth})
		if got := market.nextSubscribed(t); got != tierChannel(tier) {
			t.Fatalf("depth %d subscribed %q", depth, got)
		}
		first := nextTierView(t, sub, fmt.Sprintf("first view at depth %d", depth))
		requests := market.snapshotRequests()
		if got := requests[len(requests)-1]; got != fmt.Sprintf("DEPTH_%d", tier) {
			t.Fatalf("depth %d snapshot %s", depth, got)
		}
		if first.Depth != depth ||
			!slices.Equal(viewTicks(first.Bids), ladder(tierBestBid, -tierUnit, depth)) ||
			!slices.Equal(viewTicks(first.Asks), ladder(tierBestAsk, tierUnit, depth)) {
			t.Fatalf("depth %d first view has depth %d, %d bids, %d asks", depth, first.Depth, len(first.Bids), len(first.Asks))
		}
		before := market.deliveredTo(tier)
		qty++
		market.publish(t, func(m *tierMarket) { m.bids[tierBestBid] = qty })
		updated := nextTierView(t, sub, fmt.Sprintf("view after a delta at depth %d", depth))
		if market.deliveredTo(tier) != before+1 {
			t.Fatalf("depth %d: no delta delivered", depth)
		}
		if updated.Depth != depth || updated.Bids[0].Qty.Scaled() != qty || len(updated.Bids) != depth || len(updated.Asks) != depth {
			t.Fatalf("depth %d updated view has depth %d, %d bids, %d asks", depth, updated.Depth, len(updated.Bids), len(updated.Asks))
		}
		sub.Close()
		// Sockets of closed subscriptions must not count as later delivery.
		market.forgetConns()
	}
	if len(seen) != len(publishedTiers) {
		t.Fatalf("tiers exercised: %v", seen)
	}
	if got := market.connections.Load(); got != 500 {
		t.Fatalf("connections = %d, want 500", got)
	}
}

func TestManagedBookEachTierUsesItselfForChannelSnapshotAndView(t *testing.T) {
	for _, tier := range publishedTiers {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			service, market := newTierService(t, 600)
			sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: tier})
			if got := market.nextSubscribed(t); got != tierChannel(tier) {
				t.Fatalf("subscribed %q", got)
			}
			view := nextTierView(t, sub, "first view")
			assertSnapshots(t, market, fmt.Sprintf("DEPTH_%d", tier))
			if view.Depth != tier || len(view.Bids) != tier || len(view.Asks) != tier {
				t.Fatalf("view has depth %d, %d bids, %d asks", view.Depth, len(view.Bids), len(view.Asks))
			}
			market.publish(t, func(m *tierMarket) { m.asks[tierBestAsk] = 9 })
			if got := nextTierView(t, sub, "view after a delta").Asks[0].Qty.Scaled(); got != 9 {
				t.Fatalf("ask qty = %d", got)
			}
		})
	}
}

func TestManagedBookUnsetDepthStaysFifty(t *testing.T) {
	service, market := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{})
	if got := market.nextSubscribed(t); got != tierChannel(50) {
		t.Fatalf("subscribed %q", got)
	}
	if view := nextTierView(t, sub, "first view"); view.Depth != 50 {
		t.Fatalf("depth = %d", view.Depth)
	}
}

func TestManagedBookUnservableDepthRefusedBeforeConnecting(t *testing.T) {
	for _, depth := range []int{501, 1000, 5000, -1} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			service, market := newTierService(t, 600)
			_, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: depth})
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(err.Error(), "depth") {
				t.Fatalf("err = %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			if got := market.connections.Load(); got != 0 {
				t.Fatalf("connections = %d", got)
			}
			assertSnapshots(t, market)

			// Control: the same service does reach this server at the top tier.
			sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 500})
			nextTierView(t, sub, "first view")
			if got := market.connections.Load(); got != 1 {
				t.Fatalf("control connections = %d", got)
			}
			assertSnapshots(t, market, "DEPTH_500")
		})
	}
}

func TestManagedBookRefusalAboveTopTierNamesLimitAndSnapshot(t *testing.T) {
	service, _ := newTierService(t, 600)
	_, err := service.CreateSubscription(context.Background(), services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 1000})
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "Orderbook.Get") {
		t.Fatalf("err = %v", err)
	}
}

// Fails if the snapshot is taken at the requested depth: level 11 would be unknown.
func TestManagedBookTrimmedViewPromotesNextLevelAfterRemoval(t *testing.T) {
	service, market := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10})
	first := nextTierView(t, sub, "first view")
	if got := first.Bids[9].Price.Ticks(); got != tierBestBid-9*tierUnit {
		t.Fatalf("tenth bid = %d", got)
	}
	market.publish(t, func(m *tierMarket) { delete(m.bids, tierBestBid) })
	updated := nextTierView(t, sub, "view after a removal")
	if want := ladder(tierBestBid-tierUnit, -tierUnit, 10); !slices.Equal(viewTicks(updated.Bids), want) {
		t.Fatalf("bids = %v, want %v", viewTicks(updated.Bids), want)
	}
}

func TestManagedBookLevelsOutsideViewStayTracked(t *testing.T) {
	service, market := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10})
	nextTierView(t, sub, "first view")
	deep := tierBestBid - 14*tierUnit
	market.publish(t, func(m *tierMarket) { m.bids[deep] = 33 })
	quiet := nextTierView(t, sub, "view after a deep change")
	if slices.Contains(viewTicks(quiet.Bids), deep) {
		t.Fatal("a level outside the requested depth is in the view")
	}
	if quiet.BookSeq != "101" {
		t.Fatalf("seq = %s", quiet.BookSeq)
	}
	market.publish(t, func(m *tierMarket) {
		for i := range int64(5) {
			delete(m.bids, tierBestBid-i*tierUnit)
		}
	})
	promoted := nextTierView(t, sub, "view after removals")
	index := slices.Index(viewTicks(promoted.Bids), deep)
	if index < 0 || promoted.Bids[index].Qty.Scaled() != 33 {
		t.Fatalf("bids = %v qtys = %v", viewTicks(promoted.Bids), viewQtys(promoted.Bids))
	}
}

func gapCounter() (func(), func() int64) {
	var gaps atomic.Int64
	return func() { gaps.Add(1) }, gaps.Load
}

func TestManagedBookGapInDeltaOutsideViewRefreshesAtTier(t *testing.T) {
	service, market := newTierService(t, 600)
	onGap, gaps := gapCounter()
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, OnSequenceGap: onGap})
	nextTierView(t, sub, "first view")
	deep := tierBestBid - 14*tierUnit
	// Sequences from the snapshot's 100, skipping ahead; only level 15 changes.
	market.update(func(m *tierMarket) { m.bids[deep] = 44; m.seq = 200 })
	market.send(t, 20, tierDelta{start: 150, end: 200, bids: map[int64]int64{deep: 44}})
	refreshed := nextTierView(t, sub, "view after the gap refresh")
	if gaps() != 1 {
		t.Fatalf("gaps = %d", gaps())
	}
	assertSnapshots(t, market, "DEPTH_20", "DEPTH_20")
	if refreshed.BookSeq != "200" || len(refreshed.Bids) != 10 {
		t.Fatalf("seq %s with %d bids", refreshed.BookSeq, len(refreshed.Bids))
	}
}

func TestManagedBookSequenceZeroSnapshotDetectsGapAtTrimmedDepth(t *testing.T) {
	service, market := newTierService(t, 600)
	market.update(func(m *tierMarket) { m.seq = 0 })
	onGap, gaps := gapCounter()
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, OnSequenceGap: onGap})
	first := nextTierView(t, sub, "first view")
	if first.BookSeq != "0" || len(first.Bids) != 10 {
		t.Fatalf("first view seq %s with %d bids", first.BookSeq, len(first.Bids))
	}
	market.update(func(m *tierMarket) { m.seq = 6 })
	market.send(t, 20, tierDelta{start: 5, end: 6, bids: map[int64]int64{tierBestBid: 8}})
	refreshed := nextTierView(t, sub, "view after the gap refresh")
	if gaps() != 1 {
		t.Fatalf("gaps = %d", gaps())
	}
	if refreshed.BookSeq != "6" || refreshed.Bids[0].Qty.Scaled() != 5 {
		t.Fatalf("seq %s qty %d: the gapped delta must not be applied", refreshed.BookSeq, refreshed.Bids[0].Qty.Scaled())
	}
}

func TestManagedBookSequenceZeroSnapshotAppliesNextContiguousDelta(t *testing.T) {
	service, market := newTierService(t, 600)
	market.update(func(m *tierMarket) { m.seq = 0 })
	onGap, gaps := gapCounter()
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, OnSequenceGap: onGap})
	nextTierView(t, sub, "first view")
	market.send(t, 20, tierDelta{start: 1, end: 1, bids: map[int64]int64{tierBestBid: 8}})
	updated := nextTierView(t, sub, "view after a delta")
	if gaps() != 0 || updated.BookSeq != "1" || updated.Bids[0].Qty.Scaled() != 8 {
		t.Fatalf("gaps %d seq %s qty %d", gaps(), updated.BookSeq, updated.Bids[0].Qty.Scaled())
	}
	assertSnapshots(t, market, "DEPTH_20")
}

func TestManagedBookUnavailableStaysUnknownThenRecoversTrimmed(t *testing.T) {
	service, market := newTierService(t, 600)
	market.update(func(m *tierMarket) { m.unavailable = true })
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10})
	unknown := nextTierView(t, sub, "first view")
	if unknown.Available || unknown.Depth != 10 || len(unknown.Bids) != 0 {
		t.Fatalf("first view %+v", unknown)
	}
	// A reset delta carries no availability: the view stays unavailable.
	market.send(t, 20, tierDelta{start: 0, end: 101, reset: true, bids: map[int64]int64{tierBestBid: 5}})
	if still := nextTierView(t, sub, "view after a reset"); still.Available {
		t.Fatal("a delta made the book available")
	}
	market.update(func(m *tierMarket) { m.unavailable = false })
	recovered := nextTierView(t, sub, "view after the book came back")
	if !recovered.Available || len(recovered.Bids) != 10 || len(recovered.Asks) != 10 {
		t.Fatalf("recovered view has available %v, %d bids, %d asks", recovered.Available, len(recovered.Bids), len(recovered.Asks))
	}
	assertSnapshots(t, market, "DEPTH_20", "DEPTH_20")
}

func TestManagedBookResetDeltaReplacesBookAndStaysTrimmed(t *testing.T) {
	service, market := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 2})
	nextTierView(t, sub, "first view")
	prices := map[int64]int64{}
	for i := range int64(20) {
		prices[tierBestBid-i*tierUnit] = 3
	}
	market.send(t, 20, tierDelta{start: 0, end: 500, reset: true, bids: prices})
	view := nextTierView(t, sub, "view after a reset")
	if !slices.Equal(viewTicks(view.Bids), []int64{tierBestBid, tierBestBid - tierUnit}) || len(view.Asks) != 0 || view.BookSeq != "500" {
		t.Fatalf("view %v asks %d seq %s", viewTicks(view.Bids), len(view.Asks), view.BookSeq)
	}
}

func TestManagedBookCoarseBucketAggregatesTierThenTrims(t *testing.T) {
	service, market := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 2, Bucket: "5"})
	view := nextTierView(t, sub, "first view")
	// Bids floor to 2000 (one level) and 1995 (five levels); asks ceil to 2005 and 2010.
	if !slices.Equal(viewTicks(view.Bids), []int64{2000 * tierUnit, 1995 * tierUnit}) || !slices.Equal(viewQtys(view.Bids), []int64{5, 25}) {
		t.Fatalf("bids %v qtys %v", viewTicks(view.Bids), viewQtys(view.Bids))
	}
	if !slices.Equal(viewTicks(view.Asks), []int64{2005 * tierUnit, 2010 * tierUnit}) || !slices.Equal(viewQtys(view.Asks), []int64{25, 25}) {
		t.Fatalf("asks %v qtys %v", viewTicks(view.Asks), viewQtys(view.Asks))
	}
	market.publish(t, func(m *tierMarket) { m.bids[tierBestBid-tierUnit] = 15 })
	if got := viewQtys(nextTierView(t, sub, "view after a delta").Bids); !slices.Equal(got, []int64{5, 35}) {
		t.Fatalf("qtys %v", got)
	}
}

func TestManagedBookCoarseBucketAbovePriceShowsZeroBidAtTrimmedDepth(t *testing.T) {
	service, _ := newTierService(t, 600)
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, Bucket: "100000"})
	view := nextTierView(t, sub, "first view")
	if !slices.Equal(viewTicks(view.Bids), []int64{0}) || view.Bids[0].Qty.Scaled() != 5*20 {
		t.Fatalf("bids %v qtys %v: every tier level floors into the zero bucket", viewTicks(view.Bids), viewQtys(view.Bids))
	}
	if !slices.Equal(viewTicks(view.Asks), []int64{100000 * tierUnit}) {
		t.Fatalf("asks %v", viewTicks(view.Asks))
	}
}

func TestManagedBookZeroPriceOutsideTrimmedViewStillClosesStream(t *testing.T) {
	service, market := newTierService(t, 600)
	var reported atomic.Int64
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, OnError: func(error) { reported.Add(1) }})
	nextTierView(t, sub, "first view")
	market.send(t, 20, tierDelta{start: 100, end: 101, bids: map[int64]int64{0: 5}})
	select {
	case view, open := <-sub.Updates():
		if open {
			t.Fatalf("a view after the bad level: %+v", view)
		}
	case <-time.After(tierWait):
		t.Fatal("stream stayed open after a zero-price level")
	}
	if sub.Err() == nil || !strings.Contains(sub.Err().Error(), "invalid or missing price") {
		t.Fatalf("Err() = %v", sub.Err())
	}
	if reported.Load() != 1 {
		t.Fatalf("errors reported = %d", reported.Load())
	}
}

func TestManagedBookReconnectStaysOnTierAndRefetchesAtIt(t *testing.T) {
	service, market := newTierService(t, 600)
	var reconnects atomic.Int64
	sub := tierBook(t, service, services.CreateSubscriptionOptions{Depth: 10, OnReconnect: func() { reconnects.Add(1) }})
	if got := market.nextSubscribed(t); got != tierChannel(20) {
		t.Fatalf("subscribed %q", got)
	}
	nextTierView(t, sub, "first view")
	market.dropConns(tierChannel(20))
	select {
	case got := <-market.subscribed:
		if got != tierChannel(20) {
			t.Fatalf("resubscribed %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no resubscribe after the connection dropped")
	}
	refreshed := nextTierView(t, sub, "view after the reconnect")
	if reconnects.Load() != 1 {
		t.Fatalf("reconnects = %d", reconnects.Load())
	}
	assertSnapshots(t, market, "DEPTH_20", "DEPTH_20")
	if len(refreshed.Bids) != 10 {
		t.Fatalf("%d bids", len(refreshed.Bids))
	}
}

func TestRawDeltasAtPublishedTierAreDelivered(t *testing.T) {
	for _, tier := range publishedTiers {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			service, market := newTierService(t, 600)
			sub, err := service.SubscribeDeltas(context.Background(), 1, tier)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			if got := market.nextSubscribed(t); got != tierChannel(tier) {
				t.Fatalf("subscribed %q", got)
			}
			market.publish(t, func(m *tierMarket) { m.bids[tierBestBid] = 7 })
			select {
			case delta := <-sub.Messages():
				if delta.BookSeqEnd != "101" || delta.BookSeqStart != "100" || len(delta.Bids) != 1 || delta.Bids[0].QtyScaled != 7 {
					t.Fatalf("delta %+v", delta)
				}
			case <-time.After(tierWait):
				t.Fatal("no raw delta on a published tier")
			}
			if market.deliveredTo(tier) != 1 {
				t.Fatal("no delta was delivered")
			}
		})
	}
}

func TestRawDeltasAtUnpublishedDepthAreRefused(t *testing.T) {
	for _, depth := range []int{5, 10, 25, 100, 501, 1000, 0, -1} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			service, market := newTierService(t, 600)
			_, err := service.SubscribeDeltas(context.Background(), 1, depth)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(err.Error(), "1, 20, 50, 200, 500") {
				t.Fatalf("err = %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			if got := market.connections.Load(); got != 0 {
				t.Fatalf("connections = %d", got)
			}
		})
	}
}

func TestRawSnapshotServesEveryEnumDepth(t *testing.T) {
	for _, depth := range []int{1, 5, 10, 20, 50, 100, 200, 500, 1000} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			service, market := newTierService(t, 1100)
			book, err := service.Get(context.Background(), "BTC-USDT", depth)
			if err != nil {
				t.Fatal(err)
			}
			assertSnapshots(t, market, fmt.Sprintf("DEPTH_%d", depth))
			if book.Depth != depth || len(book.Bids) != depth || len(book.Asks) != depth {
				t.Fatalf("book has depth %d, %d bids, %d asks", book.Depth, len(book.Bids), len(book.Asks))
			}
			if got := market.connections.Load(); got != 0 {
				t.Fatalf("connections = %d", got)
			}
		})
	}
}
