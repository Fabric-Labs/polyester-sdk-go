package codecs

import (
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestBatchCreateOrdersToProto(t *testing.T) {
	symbol := "BTC-USD"
	tif := "gtc"
	cid := "cid-1"
	price := models.PriceFromDecimal("50000")
	items := []models.CreateOrderRequest{
		{Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif, Qty: models.QtyFromDecimal("0.1"), Price: &price, ClientOrderID: &cid},
		{Symbol: strPtr("ETH-USD"), Side: "sell", OrderType: "market", Qty: models.QtyFromDecimal("1"), ClientOrderID: strPtr("cid-2")},
	}
	reqID := "req-create-1"
	proto, err := BatchCreateOrdersToProto(items, strPtr("123"), &reqID, true, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if proto.RequestId != "req-create-1" {
		t.Fatalf("request meta: %+v", proto)
	}
	if proto.GetSubaccountId() != 123 || len(proto.Items) != 2 {
		t.Fatalf("items=%d sub=%d", len(proto.Items), proto.GetSubaccountId())
	}
	if proto.Items[0].Symbol != "BTC-USD" || proto.Items[0].Side != orderv1.Side_BUY {
		t.Fatalf("item0=%+v", proto.Items[0])
	}
	limitGtc := proto.Items[0].GetLimitGtc()
	if limitGtc == nil {
		t.Fatalf("item0 expected limit_gtc execution: %+v", proto.Items[0])
	}
	if proto.Items[0].GetBaseQtyScaled() != 10_000_000 || limitGtc.GetPriceTicks() != 50_000_000_000 {
		t.Fatalf("item0 scales: qty=%d price=%d", proto.Items[0].GetBaseQtyScaled(), limitGtc.GetPriceTicks())
	}
	if proto.Items[1].GetMarketIoc() == nil {
		t.Fatalf("item1 expected market_ioc execution: %+v", proto.Items[1])
	}
}

func TestBatchCreateOrdersRequiresItems(t *testing.T) {
	_, err := BatchCreateOrdersToProto(nil, nil, nil, false, 8, 6)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBatchSizeGuardRejectsMoreThanTwenty(t *testing.T) {
	symbol := "BTC-USD"
	tif := "gtc"
	price := models.PriceFromDecimal("50000")
	items := make([]models.CreateOrderRequest, 21)
	for i := range items {
		items[i] = models.CreateOrderRequest{
			Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
			Qty: models.QtyFromDecimal("0.1"), Price: &price,
		}
	}
	if _, err := BatchCreateOrdersToProto(items, nil, nil, false, 8, 6); err == nil {
		t.Fatal("expected batch_create max-20 rejection")
	}
	cancelItems := make([]models.BatchCancelItem, 21)
	for i := range cancelItems {
		cancelItems[i] = models.BatchCancelItem{Key: models.OrderKeyByID("1")}
	}
	if _, err := BatchCancelOrdersToProto(cancelItems, nil, nil); err == nil {
		t.Fatal("expected batch_cancel max-20 rejection")
	}
	replaceItems := make([]models.BatchReplaceItem, 21)
	for i := range replaceItems {
		p := models.PriceFromDecimal("1")
		replaceItems[i] = models.BatchReplaceItem{Key: models.OrderKeyByID("1"), NewPrice: &p}
	}
	if _, err := BatchReplaceOrdersToProto(replaceItems, 1, nil, nil, 8); err == nil {
		t.Fatal("expected batch_replace max-20 rejection")
	}
}

func TestBatchCancelOrdersToProto(t *testing.T) {
	sid := uint32(3)
	sid2 := uint32(5)
	items := []models.BatchCancelItem{
		{Key: models.OrderKeyByID("100"), SymbolID: &sid},
		{Key: models.OrderKeyByClientID("cid-9"), SymbolID: &sid2},
	}
	reqID := "req-cancel-1"
	proto, err := BatchCancelOrdersToProto(items, strPtr("100"), &reqID)
	if err != nil {
		t.Fatal(err)
	}
	if proto.RequestId != "req-cancel-1" || proto.GetSubaccountId() != 100 || len(proto.Items) != 2 {
		t.Fatalf("proto=%+v", proto)
	}
	if proto.Items[0].OrderId != 100 || proto.Items[0].SymbolId != 3 {
		t.Fatalf("item0=%+v", proto.Items[0])
	}
	if proto.Items[1].ClientOrderId != "cid-9" || proto.Items[1].SymbolId != 5 {
		t.Fatalf("item1=%+v", proto.Items[1])
	}
}

func TestBatchCancelItemRequiresOneTarget(t *testing.T) {
	_, err := BatchCancelOrdersToProto([]models.BatchCancelItem{{}}, nil, nil)
	if err == nil {
		t.Fatal("expected validation error for empty item")
	}
}

func strPtr(s string) *string { return &s }
