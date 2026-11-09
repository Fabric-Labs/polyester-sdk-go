package services

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// batchReceiptHandler answers the batch writes with fixed receipts and counts
// the requests it receives.
type batchReceiptHandler struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
	mu      sync.Mutex
	calls   int
	create  *orderv1.BatchCreateOrdersResponse
	cancel  *orderv1.BatchCancelOrdersResponse
	replace *orderv1.BatchReplaceOrdersResponse
}

func (h *batchReceiptHandler) BatchCreateOrders(context.Context, *connect.Request[orderv1.BatchCreateOrdersRequest]) (*connect.Response[orderv1.BatchCreateOrdersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	return connect.NewResponse(h.create), nil
}

func (h *batchReceiptHandler) BatchCancelOrders(context.Context, *connect.Request[orderv1.BatchCancelOrdersRequest]) (*connect.Response[orderv1.BatchCancelOrdersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	return connect.NewResponse(h.cancel), nil
}

func (h *batchReceiptHandler) BatchReplaceOrders(context.Context, *connect.Request[orderv1.BatchReplaceOrdersRequest]) (*connect.Response[orderv1.BatchReplaceOrdersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	return connect.NewResponse(h.replace), nil
}

func newBatchReceiptService(t *testing.T, h *batchReceiptHandler, wire string) *OrdersService {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := ordersv1connect.NewOrdersServiceHandler(h)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	return NewOrdersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

// publicFields renders a public result the way a caller serialises it.
func publicFields(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func violationFields(field, rule, message string) []any {
	return []any{map[string]any{"field_path": field, "rule_id": rule, "message": message}}
}

const (
	tickField   = "order.attached_risk.take_profit.trigger_price_ticks"
	tickMessage = "Take-profit trigger price must be a multiple of the tick size."
	qtyField    = "order.base_qty_scaled"
	qtyMessage  = "Quantity is below the minimum."
)

func tickViolation() []*orderv1.FieldViolation {
	return []*orderv1.FieldViolation{{FieldPath: tickField, RuleId: "tick_size", Message: tickMessage}}
}

func qtyViolation() []*orderv1.FieldViolation {
	return []*orderv1.FieldViolation{{FieldPath: qtyField, RuleId: "minimum", Message: qtyMessage}}
}

// POLY-8960: each rejected batch item keeps the field violations the server
// returned, in item order, beside the accepted IDs.
func TestBatchCreateKeepsEachRejectionsFieldViolations(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &batchReceiptHandler{create: &orderv1.BatchCreateOrdersResponse{
				Results: []*orderv1.BatchCreateResultItem{
					{ClientOrderId: "passive", Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{OrderId: 42}}},
					{ClientOrderId: "off-tick", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{Error: &orderv1.ErrorDetail{
						Code: orderv1.ErrorCode_ERROR_CODE_PRICE_TICK_SIZE, Violations: tickViolation(),
					}}}},
					{ClientOrderId: "tiny", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{Error: &orderv1.ErrorDetail{
						Code: orderv1.ErrorCode_ERROR_CODE_MIN_QTY, Violations: qtyViolation(),
					}}}},
					{ClientOrderId: "plain", Outcome: &orderv1.BatchCreateResultItem_Rejected{Rejected: &orderv1.BatchCreateRejected{Error: &orderv1.ErrorDetail{
						Code: orderv1.ErrorCode_ERROR_CODE_BAD_PRICE,
					}}}},
				},
				AcceptedCount: 1,
				RejectedCount: 3,
			}}
			service := newBatchReceiptService(t, h, wire)
			symbolID := uint32(1)
			price := models.PriceFromDecimal("100")
			items := make([]models.CreateOrderRequest, 4)
			for i := range items {
				items[i] = models.CreateOrderRequest{SymbolID: &symbolID, Side: "buy", OrderType: "limit", Price: &price, Qty: models.QtyFromDecimal("0.001")}
			}
			result, err := service.BatchCreate(context.Background(), nil, items, nil, nil, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if h.calls != 1 || result.AcceptedCount != 1 || result.RejectedCount != 3 || len(result.Results) != 4 {
				t.Fatalf("calls=%d result=%+v", h.calls, result)
			}
			want := []map[string]any{
				{"status": "accepted", "order_id": codecs.FormatID(42), "client_order_id": "passive"},
				{"status": "rejected", "client_order_id": "off-tick", "code": "ERROR_CODE_PRICE_TICK_SIZE", "violations": violationFields(tickField, "tick_size", tickMessage)},
				{"status": "rejected", "client_order_id": "tiny", "code": "ERROR_CODE_MIN_QTY", "violations": violationFields(qtyField, "minimum", qtyMessage)},
				{"status": "rejected", "client_order_id": "plain", "code": "ERROR_CODE_BAD_PRICE"},
			}
			for i, item := range result.Results {
				if got := publicFields(t, item); !reflect.DeepEqual(got, want[i]) {
					t.Errorf("item %d:\n got %v\nwant %v", i, got, want[i])
				}
			}
		})
	}
}

// POLY-8960: batch cancel and batch replace items carry the same ErrorDetail,
// so their rejections keep its field violations too.
func TestBatchCancelAndReplaceKeepEachRejectionsFieldViolations(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &batchReceiptHandler{
				cancel: &orderv1.BatchCancelOrdersResponse{
					Results: []*orderv1.BatchCancelResultItem{
						{Status: orderv1.BatchCancelResultItem_ACCEPTED, OrderId: 42},
						{Status: orderv1.BatchCancelResultItem_REJECTED, OrderId: 45, Code: "INVALID_ARGUMENT", Error: &orderv1.ErrorDetail{
							Code: orderv1.ErrorCode_ERROR_CODE_MIN_QTY, Violations: qtyViolation(),
						}},
					},
					AcceptedCount: 1,
					RejectedCount: 1,
				},
				replace: &orderv1.BatchReplaceOrdersResponse{
					BatchRequestId: 9,
					Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
					Results: []*orderv1.BatchReplaceAdmissionItem{
						{ItemIndex: 0, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED, OldOrderId: 42, ReplacementOrderId: 43, ActionTaken: orderv1.ModifyActionTaken_REPLACED},
						{ItemIndex: 1, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED, OldOrderId: 44, Code: "PRICE_TICK_SIZE", Error: &orderv1.ErrorDetail{
							Code: orderv1.ErrorCode_ERROR_CODE_PRICE_TICK_SIZE, Violations: tickViolation(),
						}},
					},
					AcceptedCount: 1,
					RejectedCount: 1,
				},
			}
			service := newBatchReceiptService(t, h, wire)
			canceled, err := service.BatchCancel(context.Background(), nil, []models.BatchCancelItem{
				{Key: models.OrderKeyByID(codecs.FormatID(42))}, {Key: models.OrderKeyByID(codecs.FormatID(45))},
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantCancel := []map[string]any{
				{"status": "accepted", "order_id": codecs.FormatID(42)},
				{"status": "rejected", "order_id": codecs.FormatID(45), "code": "INVALID_ARGUMENT", "violations": violationFields(qtyField, "minimum", qtyMessage)},
			}
			for i, item := range canceled.Results {
				if got := publicFields(t, item); !reflect.DeepEqual(got, wantCancel[i]) {
					t.Errorf("cancel item %d:\n got %v\nwant %v", i, got, wantCancel[i])
				}
			}
			price := models.PriceFromDecimal("101")
			replaced, err := service.BatchReplace(context.Background(), nil, []models.BatchReplaceItem{
				{Key: models.OrderKeyByID(codecs.FormatID(42)), NewPrice: &price}, {Key: models.OrderKeyByID(codecs.FormatID(44)), NewPrice: &price},
			}, "BTC-USDT", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantReplace := []map[string]any{
				{"item_index": float64(0), "status": "admitted", "old_order_id": codecs.FormatID(42), "replacement_order_id": codecs.FormatID(43), "action_taken": "replaced"},
				{"item_index": float64(1), "status": "rejected", "old_order_id": codecs.FormatID(44), "code": "PRICE_TICK_SIZE", "violations": violationFields(tickField, "tick_size", tickMessage)},
			}
			for i, item := range replaced.Results {
				if got := publicFields(t, item); !reflect.DeepEqual(got, wantReplace[i]) {
					t.Errorf("replace item %d:\n got %v\nwant %v", i, got, wantReplace[i])
				}
			}
			if h.calls != 2 {
				t.Fatalf("calls=%d", h.calls)
			}
		})
	}
}

// POLY-8920: a rejected batch cancel that resolved no order keeps its order
// ID empty instead of reporting order "0"; a resolved ID is kept as sent.
func TestBatchCancelKeepsAnUnresolvedOrderIDEmpty(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			h := &batchReceiptHandler{cancel: &orderv1.BatchCancelOrdersResponse{
				Results: []*orderv1.BatchCancelResultItem{
					{Status: orderv1.BatchCancelResultItem_ACCEPTED, OrderId: 42},
					{Status: orderv1.BatchCancelResultItem_REJECTED, ClientOrderId: "missing", Code: "ORDER_UNKNOWN"},
					{Status: orderv1.BatchCancelResultItem_REJECTED, OrderId: 77, Code: "ORDER_NOT_OPEN"},
				},
				AcceptedCount: 1,
				RejectedCount: 2,
			}}
			service := newBatchReceiptService(t, h, wire)
			result, err := service.BatchCancel(context.Background(), nil, []models.BatchCancelItem{
				{Key: models.OrderKeyByID(codecs.FormatID(42))}, {Key: models.OrderKeyByClientID("missing")}, {Key: models.OrderKeyByID(codecs.FormatID(77))},
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if h.calls != 1 || result.AcceptedCount != 1 || result.RejectedCount != 2 || len(result.Results) != 3 {
				t.Fatalf("calls=%d result=%+v", h.calls, result)
			}
			got := []string{result.Results[0].OrderID, result.Results[1].OrderID, result.Results[2].OrderID}
			if want := []string{codecs.FormatID(42), "", codecs.FormatID(77)}; !reflect.DeepEqual(got, want) {
				t.Fatalf("order IDs=%q want %q", got, want)
			}
			if result.Results[1].ClientOrderID != "missing" || result.Results[1].Code != "ORDER_UNKNOWN" {
				t.Fatalf("rejection=%+v", result.Results[1])
			}
		})
	}
}
