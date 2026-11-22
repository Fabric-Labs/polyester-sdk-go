package services

import (
	"context"
	"testing"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Each accepted batch item carries the base quantity the server resolved, at
// its pair's base quantity scale as a single create does, and the batch keeps
// the server's response time. A rejected item has no resolved quantity.
func TestBatchCreateItemsCarryResolvedBaseQtyAndResponseTime(t *testing.T) {
	quoteBudget := int64(500_000_000)
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &batchReceiptHandler{create: &orderv1.BatchCreateOrdersResponse{
				Results: []*orderv1.BatchCreateResultItem{
					{ClientOrderId: "post-only", Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{
						OrderId: 42, ResolvedBaseQtyScaled: 1_000_000,
					}}},
					{ClientOrderId: "budget", Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{
						OrderId: 43, ResolvedBaseQtyScaled: 775_691, SubmittedMaxQuoteDebitScaled: &quoteBudget,
					}}},
					{ClientOrderId: "refused", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{Error: &orderv1.ErrorDetail{
						Code: orderv1.ErrorCode_ERROR_CODE_MIN_QTY,
					}}}},
				},
				AcceptedCount: 2,
				RejectedCount: 1,
				Ts:            &timestamppb.Timestamp{Seconds: 1_763_170_000, Nanos: 5},
				TsNs:          1_763_170_000_123_456_789,
			}}
			service := newBatchReceiptService(t, h, wire)
			symbolID := uint32(1)
			price := models.PriceFromDecimal("100")
			items := make([]models.CreateOrderRequest, 3)
			for i := range items {
				items[i] = models.CreateOrderRequest{SymbolID: &symbolID, Side: "buy", OrderType: "limit", Price: &price, Qty: models.QtyFromDecimal("0.01")}
			}
			result, err := service.BatchCreate(context.Background(), nil, items, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.TsNs != 1_763_170_000_123_456_789 {
				t.Fatalf("TsNs = %d", result.TsNs)
			}
			for i, want := range []struct{ scaled, formatted string }{{"1000000", "0.01"}, {"775691", "0.00775691"}} {
				item := result.Results[i]
				if item.ResolvedBaseQtyScaled != want.scaled || item.ResolvedBaseQty == nil {
					t.Fatalf("item %d: resolved %q %v", i, item.ResolvedBaseQtyScaled, item.ResolvedBaseQty)
				}
				got, err := item.ResolvedBaseQty.Format()
				if err != nil || got != want.formatted {
					t.Fatalf("item %d: ResolvedBaseQty.Format() = %q, %v; want %q", i, got, err, want.formatted)
				}
				if id := item.ResolvedBaseQty.SymbolID(); id == nil || *id != symbolID {
					t.Fatalf("item %d: resolved qty symbol ID %v", i, id)
				}
			}
			if rejected := result.Results[2]; rejected.ResolvedBaseQty != nil || rejected.ResolvedBaseQtyScaled != "" {
				t.Fatalf("rejected item has a resolved quantity: %+v", rejected)
			}
		})
	}
}
