package services

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// refusedReplacementReply is GetOrder of a refused batch replacement's
// successor within 60 seconds of the batch's acceptance, as the API sends it:
// REJECTED at version 1 with its batch and its own lineage, no terminal time
// and no order terms (no symbol, side, type, time in force, self-trade
// prevention mode or fee asset), its quantities and price 0 placeholders.
const refusedReplacementReply = `{"order":{` +
	`"orderId":"2001504781434687193",` +
	`"status":"REJECTED",` +
	`"origQtyScaled":"0","cumQtyScaled":"0","inheritedCumQtyScaled":"0","leavesQtyScaled":"0",` +
	`"priceTicks":"0","avgPriceTicks":"0",` +
	`"createdTsNs":"1763600000123456789",` +
	`"rejectReason":"ERROR_CODE_MODIFY_APPLY_EXPIRED",` +
	`"version":1,` +
	`"batchRequestId":"2001504781434687001",` +
	`"lineage":{"id":"2001504781434687193","generation":1}` +
	`},"executionsComplete":true}`

func newRefusedReplacementOrders(t *testing.T, wire string) *OrdersService {
	t.Helper()
	var reply orders.GetOrderResponse
	if err := protojson.Unmarshal([]byte(refusedReplacementReply), &reply); err != nil {
		t.Fatal(err)
	}
	binary, err := proto.Marshal(&reply)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(ordersv1connect.OrdersReadServiceGetOrderProcedure, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(refusedReplacementReply))
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(binary)
	})
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
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)}}}); err != nil {
		t.Fatal(err)
	}
	return NewOrdersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

// Orders.Get decodes a refused batch replacement's successor, which carries no
// order terms, without error: its pair, side, type, time in force,
// self-trade prevention mode and fee asset read unset, and its placeholder
// quantities name no pair.
func TestGetOrderDecodesARefusedBatchReplacementWithoutTerms(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			svc := newRefusedReplacementOrders(t, wire)
			orderID := codecs.FormatID(2001504781434687193)
			got, err := svc.Get(context.Background(), nil, models.OrderKeyByID(orderID), nil, false, false)
			if err != nil {
				t.Fatal(err)
			}
			o := got.Order
			if o == nil {
				t.Fatal("order missing")
			}
			if o.OrderID != orderID || o.Status != "rejected" || o.RejectReason != "modify_apply_expired" ||
				o.Version != 1 || o.BatchRequestID != codecs.FormatID(2001504781434687001) || o.CreatedTsNs != "1763600000123456789" || o.TerminalTsNs != "" {
				t.Fatalf("order = %+v", *o)
			}
			if o.Lineage == nil || o.Lineage.ID != orderID || o.Lineage.Generation != 1 {
				t.Fatalf("lineage = %+v", o.Lineage)
			}
			if o.SymbolID != 0 || o.Side != "" || o.OrderType != "" || o.TIF != "" || o.SelfTradePreventionMode != "" || o.FeeAsset != "" {
				t.Fatalf("terms = symbol %d side %q type %q tif %q stp %q fee asset %q, want unset",
					o.SymbolID, o.Side, o.OrderType, o.TIF, o.SelfTradePreventionMode, o.FeeAsset)
			}
			for name, q := range map[string]models.QtyScaled{"OrigQty": o.OrigQty, "CumQty": o.CumQty, "LeavesQty": o.LeavesQty} {
				if q.Scaled() != 0 || q.SymbolID() != nil || q.Scale() != nil {
					t.Errorf("%s = %d, pair %v, scale %v; want a 0 placeholder naming no pair", name, q.Scaled(), q.SymbolID(), q.Scale())
				}
			}
			if o.Price.Ticks() != 0 {
				t.Errorf("Price = %d, want the 0 placeholder", o.Price.Ticks())
			}
			encoded, err := json.Marshal(o)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"symbol_id", "side", "order_type", "tif", "self_trade_prevention_mode", "fee_asset"} {
				if strings.Contains(string(encoded), `"`+key+`"`) {
					t.Errorf("JSON %s carries %q, want it omitted", encoded, key)
				}
			}
		})
	}
}
