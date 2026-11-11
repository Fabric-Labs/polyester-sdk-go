package decode

import "github.com/Fabric-Labs/polyester-sdk-go/models"

// BaseQuantityScales returns a pair's base quantity scale by symbol ID, and
// false when it is not known, for example before the catalogs are loaded.
// catalogs.Manager.BaseQuantityScaleForSymbolID is one.
type BaseQuantityScales func(symbolID uint32) (int, bool)

// scaleQty attaches the pair's base quantity scale to a quantity read
// without one. A quantity that already has a scale, names no pair, or whose
// pair's scale is unknown is returned unchanged.
func scaleQty(q models.QtyScaled, scales BaseQuantityScales) models.QtyScaled {
	if scales == nil || q.Scale() != nil {
		return q
	}
	symbolID := q.SymbolID()
	if symbolID == nil {
		return q
	}
	if scale, ok := scales(*symbolID); ok {
		return q.WithScale(scale)
	}
	return q
}

// ScaleOrder attaches the pair's base quantity scale to an order's
// quantities: OrigQty, CumQty, InheritedCumQty, LeavesQty and FeeTotal.Qty.
func ScaleOrder(order models.Order, scales BaseQuantityScales) models.Order {
	order.OrigQty = scaleQty(order.OrigQty, scales)
	order.CumQty = scaleQty(order.CumQty, scales)
	order.InheritedCumQty = scaleQty(order.InheritedCumQty, scales)
	order.LeavesQty = scaleQty(order.LeavesQty, scales)
	if order.FeeTotal != nil {
		fee := *order.FeeTotal
		fee.Qty = scaleQty(fee.Qty, scales)
		order.FeeTotal = &fee
	}
	return order
}

// ScaleOrders applies ScaleOrder to each order in place.
func ScaleOrders(orders []models.Order, scales BaseQuantityScales) {
	for i := range orders {
		orders[i] = ScaleOrder(orders[i], scales)
	}
}

// ScaleUserTrades attaches the pair's base quantity scale to each trade's Qty
// in place.
func ScaleUserTrades(trades []models.UserTrade, scales BaseQuantityScales) {
	for i := range trades {
		trades[i].Qty = scaleQty(trades[i].Qty, scales)
	}
}

// ScaleGetOrderResult attaches the pair's base quantity scale to an order
// read's order and trades.
func ScaleGetOrderResult(result models.GetOrderResult, scales BaseQuantityScales) models.GetOrderResult {
	if result.Order != nil {
		order := ScaleOrder(*result.Order, scales)
		result.Order = &order
	}
	ScaleUserTrades(result.Trades, scales)
	return result
}

// ScaleOrderUnitEvents attaches the pair's base quantity scale to the fill
// quantity of each order event in place.
func ScaleOrderUnitEvents(events []models.OrderUnitEvent, scales BaseQuantityScales) {
	for _, event := range events {
		if event.Order != nil && event.Order.Fill != nil {
			event.Order.Fill.Qty = scaleQty(event.Order.Fill.Qty, scales)
		}
	}
}

// ScaleTrigger attaches the pair's base quantity scale to a trigger's Qty and
// to a TWAP or ladder trigger's Details.ExecutedQty.
func ScaleTrigger(trigger models.Trigger, scales BaseQuantityScales) models.Trigger {
	trigger.Qty = scaleQty(trigger.Qty, scales)
	if trigger.Details != nil {
		details := *trigger.Details
		details.ExecutedQty = scaleQty(details.ExecutedQty, scales)
		trigger.Details = &details
	}
	return trigger
}
