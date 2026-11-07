package services

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderTradesCompleteHelper(t *testing.T) {
	incomplete := models.GetOrderResult{
		Order:  &models.Order{Status: "filled", CumQty: models.MustQtyScaled(100)},
		Trades: []models.UserTrade{{Qty: models.MustQtyScaled(40)}},
	}
	if orderTradesComplete(incomplete) {
		t.Fatal("expected incomplete")
	}
	complete := models.GetOrderResult{
		Order: &models.Order{Status: "filled", CumQty: models.MustQtyScaled(100)},
		Trades: []models.UserTrade{
			{Qty: models.MustQtyScaled(40)},
			{Qty: models.MustQtyScaled(60)},
		},
	}
	if !orderTradesComplete(complete) {
		t.Fatal("expected complete")
	}
	for _, status := range []string{"", "pending", "working", "pending_cancel"} {
		open := complete
		open.Order = &models.Order{Status: status, CumQty: models.MustQtyScaled(100)}
		if orderTradesComplete(open) {
			t.Fatalf("status %q: an order that can still fill is not complete", status)
		}
	}
	for _, status := range []string{"filled", "canceled", "rejected"} {
		ended := complete
		ended.Order = &models.Order{Status: status, CumQty: models.MustQtyScaled(100)}
		if !orderTradesComplete(ended) {
			t.Fatalf("status %q: an ended order with every trade recorded is complete", status)
		}
	}
	if sumTradeQty(complete.Trades) != 100 {
		t.Fatalf("sum=%d", sumTradeQty(complete.Trades))
	}
}
