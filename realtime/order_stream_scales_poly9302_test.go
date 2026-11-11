package realtime

import (
	"sync/atomic"
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// POLY-9302: the private order stream's orders and fills carry the pair's
// base quantity scale, as the order reads do, once SetBaseQuantityScales
// names it.
func TestPoly9302OrderStreamCarriesBaseQuantityScale(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	c.SetBaseQuantityScales(func(symbolID uint32) (int, bool) { return 8, symbolID == testOrder(1, 1).GetSymbolId() })
	sub, ws := connectOrders(t, f, c)
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1))
	snapshot := expectEvent[OrderSnapshot](t, sub)
	requireStreamScale(t, "snapshot CumQty", snapshot.Orders[0].CumQty)
	requireStreamScale(t, "snapshot OrigQty", snapshot.Orders[0].OrigQty)

	filled := testOrder(1, 2)
	filled.Status = orderv1.OrderStatus_FILLED
	filled.CumQtyScaled = 10
	ws.send(t, "s1", seq+1, unitFrame("c2", []*orderv1.Order{filled}, nil,
		&privatewsv1.UnitEvent{Kind: &privatewsv1.UnitEvent_Order{Order: &privatewsv1.OrderEvent{
			OrderId: 1, Version: 2, Kind: privatewsv1.OrderEvent_FILLED,
			Fill: &privatewsv1.Fill{MatchId: 99, Ordinal: 1, Role: privatewsv1.Fill_MAKER, QtyScaled: 10, PriceTicks: 1_000_000_000},
		}}},
	))
	unit := expectEvent[OrderUnit](t, sub)
	requireStreamScale(t, "unit CumQty", unit.Orders[0].CumQty)
	requireStreamScale(t, "unit fill Qty", unit.Events[0].Order.Fill.Qty)
}

func requireStreamScale(t *testing.T, label string, q models.QtyScaled) {
	t.Helper()
	if scale := q.Scale(); scale == nil || *scale != 8 {
		t.Errorf("%s scale = %v, want 8", label, scale)
	}
}

// POLY-9302: a consumer that joins a running order stream gets the cached
// view at the scales known when it joins. The snapshot here arrives before
// the catalogs load, so the first consumer's orders carry no scale; once the
// catalogs know a pair, a later consumer's cached snapshot carries it, a zero
// scale included, while a pair the catalogs don't list stays unscaled.
func TestPoly9302LateOrderStreamConsumerGetsCachedViewAtKnownScales(t *testing.T) {
	f := newFakePrivateServer(t)
	c := newTestOrderClient(t, f, testTiming())
	var loaded atomic.Bool
	c.SetBaseQuantityScales(func(symbolID uint32) (int, bool) {
		if !loaded.Load() {
			return 0, false
		}
		switch symbolID {
		case 1:
			return 8, true
		case 2:
			return 0, true
		}
		return 0, false
	})
	first, ws := connectOrders(t, f, c)
	zero, unknown := testOrder(2, 1), testOrder(3, 1)
	zero.SymbolId, unknown.SymbolId = 2, 99
	seq := ws.snapshot(t, "s1", 1, "c1", testOrder(1, 1), zero, unknown)
	early := expectEvent[OrderSnapshot](t, first)
	for _, order := range early.Orders {
		if scale := order.CumQty.Scale(); scale != nil {
			t.Fatalf("order %s scale = %d before the catalogs loaded", order.OrderID, *scale)
		}
	}

	loaded.Store(true)
	late := subscribeOrders(t, c, testAccount)
	expectEvent[OrderStreamConnected](t, late)
	cached := expectEvent[OrderSnapshot](t, late)
	requireCachedScales(t, "late", cached.Orders, map[uint32]*int{1: intPtr(8), 2: intPtr(0), 99: nil})
	if got, err := cached.Orders[0].OrigQty.Format(); err != nil || got != "0.0000001" {
		t.Fatalf("late OrigQty.Format() = %q, %v; want 0.0000001", got, err)
	}

	// A unit applied after the join keeps the cache at the known scales.
	ws.send(t, "s1", seq+1, unitFrame("c2", []*orderv1.Order{testOrder(1, 2)}, nil))
	for _, sub := range []*Subscription[OrderStreamEvent]{first, late} {
		unit := expectEvent[OrderUnit](t, sub)
		requireStreamScale(t, "unit CumQty", unit.Orders[0].CumQty)
	}
	third := subscribeOrders(t, c, testAccount)
	expectEvent[OrderStreamConnected](t, third)
	replay := expectEvent[OrderSnapshot](t, third)
	if replay.Orders[0].Version != 2 {
		t.Fatalf("replayed version = %d, want 2", replay.Orders[0].Version)
	}
	requireCachedScales(t, "third", replay.Orders, map[uint32]*int{1: intPtr(8), 2: intPtr(0), 99: nil})
}

func requireCachedScales(t *testing.T, label string, orders []models.Order, want map[uint32]*int) {
	t.Helper()
	if len(orders) != len(want) {
		t.Fatalf("%s: %d orders, want %d", label, len(orders), len(want))
	}
	for _, order := range orders {
		expected := want[order.SymbolID]
		for name, q := range map[string]models.QtyScaled{"CumQty": order.CumQty, "OrigQty": order.OrigQty, "LeavesQty": order.LeavesQty} {
			got := q.Scale()
			switch {
			case expected == nil && got != nil:
				t.Errorf("%s symbol %d %s scale = %d, want none", label, order.SymbolID, name, *got)
			case expected != nil && (got == nil || *got != *expected):
				t.Errorf("%s symbol %d %s scale = %v, want %d", label, order.SymbolID, name, got, *expected)
			}
		}
	}
}

func intPtr(v int) *int { return &v }
