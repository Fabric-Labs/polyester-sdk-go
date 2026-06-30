package codecs

import (
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestBatchCreateOrdersToProto(t *testing.T) {
	symbol := "BTC-USD"
	price := "50000"
	tif := "gtc"
	cid := "cid-1"
	items := []models.CreateOrderRequest{
		{Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif, Qty: "0.1", Price: &price, ClientOrderID: &cid},
		{Symbol: strPtr("ETH-USD"), Side: "sell", OrderType: "market", Qty: "1", ClientOrderID: strPtr("cid-2")},
	}
	reqID := "req-create-1"
	proto, err := BatchCreateOrdersToProto(items, strPtr("123"), &reqID, true, 8)
	if err != nil {
		t.Fatal(err)
	}
	if proto.RequestId != "req-create-1" || !proto.AllowPartial {
		t.Fatalf("request meta: %+v", proto)
	}
	if proto.GetSubaccountId() != 123 || len(proto.Items) != 2 {
		t.Fatalf("items=%d sub=%d", len(proto.Items), proto.GetSubaccountId())
	}
	if proto.Items[0].Symbol != "BTC-USD" || proto.Items[0].Side != orderv1.Side_BUY {
		t.Fatalf("item0=%+v", proto.Items[0])
	}
	if proto.Items[0].QtyScaled != 10_000_000 || proto.Items[0].PriceTicks != 50_000_000_000 {
		t.Fatalf("item0 scales: qty=%d price=%d", proto.Items[0].QtyScaled, proto.Items[0].PriceTicks)
	}
	if proto.Items[1].OrderType != orderv1.OrderType_MARKET {
		t.Fatalf("item1 type=%v", proto.Items[1].OrderType)
	}
}

func TestBatchCreateOrdersRequiresItems(t *testing.T) {
	_, err := BatchCreateOrdersToProto(nil, nil, nil, false, 8)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBatchCancelOrdersToProto(t *testing.T) {
	sid := uint32(3)
	sid2 := uint32(5)
	items := []models.BatchCancelItem{
		{OrderID: strPtr("42"), SymbolID: &sid},
		{ClientOrderID: strPtr("cid-9"), SymbolID: &sid2},
	}
	reqID := "req-cancel-1"
	proto, err := BatchCancelOrdersToProto(items, strPtr("99"), &reqID)
	if err != nil {
		t.Fatal(err)
	}
	if proto.RequestId != "req-cancel-1" || proto.GetSubaccountId() != 99 || len(proto.Items) != 2 {
		t.Fatalf("proto=%+v", proto)
	}
	if proto.Items[0].OrderId != 42 || proto.Items[0].SymbolId != 3 {
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
	_, err = BatchCancelOrdersToProto([]models.BatchCancelItem{{OrderID: strPtr("1"), ClientOrderID: strPtr("cid")}}, nil, nil)
	if err == nil {
		t.Fatal("expected validation error for both targets")
	}
}

func strPtr(s string) *string { return &s }
