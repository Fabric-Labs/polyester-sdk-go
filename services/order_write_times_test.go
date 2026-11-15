package services

import (
	"context"
	"net/http"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	writeAcceptedTsNs = uint64(1_763_164_740_123_456_789)
	writeTimestampNs  = uint64(1_763_164_740_987_654_321)
)

func writeTimestamp() *timestamppb.Timestamp {
	return &timestamppb.Timestamp{Seconds: 1_763_164_740, Nanos: 987_654_321}
}

// orderWriteTimesHandler answers create with its exact acceptance time, and
// modify and batch replace with only their paired timestamps.
type orderWriteTimesHandler struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
}

func (orderWriteTimesHandler) CreateOrder(context.Context, *connect.Request[orderv1.CreateOrderRequest]) (*connect.Response[orderv1.CreateOrderResponse], error) {
	return connect.NewResponse(&orderv1.CreateOrderResponse{OrderId: 41, AcceptedAtTsNs: writeAcceptedTsNs, AcceptedAt: writeTimestamp()}), nil
}

func (orderWriteTimesHandler) ModifyOrder(context.Context, *connect.Request[orderv1.ModifyOrderRequest]) (*connect.Response[orderv1.ModifyOrderResponse], error) {
	return connect.NewResponse(&orderv1.ModifyOrderResponse{ActionTaken: orderv1.ModifyActionTaken_AMENDED, OldOrderId: 42, FinalOrderId: 42, Ts: writeTimestamp()}), nil
}

func (orderWriteTimesHandler) BatchReplaceOrders(context.Context, *connect.Request[orderv1.BatchReplaceOrdersRequest]) (*connect.Response[orderv1.BatchReplaceOrdersResponse], error) {
	existing := uint64(77)
	return connect.NewResponse(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 9,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_REJECTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{{
			ItemIndex: 0, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED,
			OldOrderId: 42, Code: "CONFLICT_DUPLICATE_CLIENT_ORDER_ID",
			Error: &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_CONFLICT_DUPLICATE_CLIENT_ORDER_ID, ExistingOrderId: &existing},
		}},
		RejectedCount: 1,
		AcceptedTs:    writeTimestamp(),
	}), nil
}

// Create, modify and batch replace keep the server's times, and a refused
// batch replace item the order holding its client order ID, over the binary
// and the JSON wire format alike.
func TestOrderWriteTimesDecodeAlikeOnBothWireFormats(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			mux := http.NewServeMux()
			path, handler := ordersv1connect.NewOrdersServiceHandler(orderWriteTimesHandler{})
			mux.Handle(path, handler)
			server := hardening.NewTestServer(mux)
			t.Cleanup(server.Close)
			creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
			if err != nil {
				t.Fatal(err)
			}
			cfg := transport.DefaultConfig()
			cfg.APIURL = server.URL
			cfg.WireFormat = transport.WireFormatFromString(wire)
			cats := catalogs.NewManager()
			if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)}}}); err != nil {
				t.Fatal(err)
			}
			svc := NewOrdersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
			ctx := context.Background()
			symbolID := uint32(1)
			price := models.PriceFromDecimal("100")
			created, err := svc.Create(ctx, models.CreateOrderRequest{SymbolID: &symbolID, Side: "buy", OrderType: "limit", Price: &price, Qty: models.QtyFromDecimal("0.01")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if created.AcceptedTsNs != writeAcceptedTsNs {
				t.Errorf("create AcceptedTsNs = %d, want %d", created.AcceptedTsNs, writeAcceptedTsNs)
			}
			newPrice := models.PriceFromDecimal("101")
			modified, err := svc.ModifyWith(ctx, nil, ModifyParams{Symbol: "BTC-USDT", Key: models.OrderKeyByID(codecs.FormatID(42)), NewPrice: &newPrice})
			if err != nil {
				t.Fatal(err)
			}
			if modified.TsNs != writeTimestampNs {
				t.Errorf("modify TsNs = %d, want %d", modified.TsNs, writeTimestampNs)
			}
			newClientOrderID := "held"
			replaced, err := svc.BatchReplace(ctx, nil, []models.BatchReplaceItem{{Key: models.OrderKeyByID(codecs.FormatID(42)), NewPrice: &newPrice, NewClientOrderID: &newClientOrderID}}, "BTC-USDT", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if replaced.AcceptedTsNs != writeTimestampNs {
				t.Errorf("batch replace AcceptedTsNs = %d, want %d", replaced.AcceptedTsNs, writeTimestampNs)
			}
			if replaced.Results[0].ExistingOrderID != codecs.FormatID(77) {
				t.Errorf("batch replace ExistingOrderID = %q", replaced.Results[0].ExistingOrderID)
			}
		})
	}
}
