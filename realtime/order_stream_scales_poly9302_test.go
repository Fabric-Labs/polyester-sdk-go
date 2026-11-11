package realtime

import (
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
