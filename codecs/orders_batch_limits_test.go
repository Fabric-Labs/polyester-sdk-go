package codecs

import (
	"strings"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func contractItemsLimit(t *testing.T, message proto.Message) int {
	t.Helper()
	field := message.ProtoReflect().Descriptor().Fields().ByName("items")
	rules, _ := proto.GetExtension(field.Options(), validate.E_Field).(*validate.FieldRules)
	return int(rules.GetRepeated().GetMaxItems())
}

func batchCancelItems(n int) []models.BatchCancelItem {
	items := make([]models.BatchCancelItem, n)
	for i := range items {
		items[i] = models.BatchCancelItem{Key: models.OrderKeyByID("1")}
	}
	return items
}

func batchReplaceItems(n int) []models.BatchReplaceItem {
	items := make([]models.BatchReplaceItem, n)
	for i := range items {
		p := models.PriceFromDecimal("1")
		items[i] = models.BatchReplaceItem{Key: models.OrderKeyByID("1"), NewPrice: &p}
	}
	return items
}

func batchCreateItems(n int) []models.CreateOrderRequest {
	symbol := "BTC-USD"
	sid := uint32(1)
	tif := "gtc"
	price := models.PriceFromDecimal("50000")
	items := make([]models.CreateOrderRequest, n)
	for i := range items {
		items[i] = models.CreateOrderRequest{
			Symbol: &symbol, SymbolID: &sid, Side: "buy", OrderType: "limit", TIF: &tif,
			Qty: models.QtyFromDecimal("0.1"), Price: &price,
		}
	}
	return items
}

func TestBatchItemLimitsMatchThePublicContract(t *testing.T) {
	for name, tc := range map[string]struct {
		message proto.Message
		want    int
	}{
		"cancel":  {&orderv1.BatchCancelOrdersRequest{}, 50},
		"replace": {&orderv1.BatchReplaceOrdersRequest{}, 50},
		"create":  {&orderv1.BatchCreateOrdersRequest{}, 20},
	} {
		if got := contractItemsLimit(t, tc.message); got != tc.want {
			t.Fatalf("%s contract max_items=%d, want %d", name, got, tc.want)
		}
	}
}

func TestBatchCancelAndReplaceAcceptUpToFifty(t *testing.T) {
	for _, n := range []int{1, 20, 21, 50} {
		req, err := BatchCancelOrdersToProto(batchCancelItems(n), nil, nil)
		if err != nil || len(req.GetItems()) != n {
			t.Fatalf("batch_cancel %d items: err=%v", n, err)
		}
		replace, err := BatchReplaceOrdersToProto(batchReplaceItems(n), 1, nil, nil, 8)
		if err != nil || len(replace.GetItems()) != n {
			t.Fatalf("batch_replace %d items: err=%v", n, err)
		}
	}
}

func TestBatchCancelAndReplaceRefuseFiftyOne(t *testing.T) {
	_, err := BatchCancelOrdersToProto(batchCancelItems(51), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "batch_cancel accepts at most 50 items") {
		t.Fatalf("batch_cancel 51 items: err=%v", err)
	}
	_, err = BatchReplaceOrdersToProto(batchReplaceItems(51), 1, nil, nil, 8)
	if err == nil || !strings.Contains(err.Error(), "batch_replace accepts at most 50 items") {
		t.Fatalf("batch_replace 51 items: err=%v", err)
	}
}

func TestBatchCreateKeepsItsTwentyItemLimit(t *testing.T) {
	if req, err := BatchCreateOrdersToProto(batchCreateItems(20), nil, nil, false, 8, 6); err != nil || len(req.GetItems()) != 20 {
		t.Fatalf("batch_create 20 items: err=%v", err)
	}
	_, err := BatchCreateOrdersToProto(batchCreateItems(21), nil, nil, false, 8, 6)
	if err == nil || !strings.Contains(err.Error(), "batch_create accepts at most 20 items") {
		t.Fatalf("batch_create 21 items: err=%v", err)
	}
}
