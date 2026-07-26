package services

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderTradesCompleteHelper(t *testing.T) {
	incomplete := models.GetOrderResult{
		Order:  &models.Order{CumQty: models.MustQtyScaled(100)},
		Trades: []models.UserTrade{{Qty: models.MustQtyScaled(40)}},
	}
	if orderTradesComplete(incomplete) {
		t.Fatal("expected incomplete")
	}
	complete := models.GetOrderResult{
		Order: &models.Order{CumQty: models.MustQtyScaled(100)},
		Trades: []models.UserTrade{
			{Qty: models.MustQtyScaled(40)},
			{Qty: models.MustQtyScaled(60)},
		},
	}
	if !orderTradesComplete(complete) {
		t.Fatal("expected complete")
	}
	if sumTradeQty(complete.Trades) != 100 {
		t.Fatalf("sum=%d", sumTradeQty(complete.Trades))
	}
}
