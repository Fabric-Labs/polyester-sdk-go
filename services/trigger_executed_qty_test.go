package services

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	triggers "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// unfilledTriggersHandler answers trigger reads with a running TWAP and
// ladder on BTC-USDT that have executed nothing yet.
type unfilledTriggersHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
}

func unfilledTwap() *triggers.Trigger {
	return &triggers.Trigger{
		TriggerId: 9, SymbolId: 1, QtyScaled: 60_000, Status: triggers.TriggerStatus_STATUS_RUNNING,
		RuntimeDetails: &triggers.Trigger_TwapState{TwapState: &triggers.TwapDetails{TwapDurationMs: 20_000, TwapSliceIntervalMs: 10_000, SliceIdx: 1, SliceCount: 2}},
	}
}

func unfilledLadder() *triggers.Trigger {
	return &triggers.Trigger{
		TriggerId: 10, SymbolId: 1, QtyScaled: 60_000, Status: triggers.TriggerStatus_STATUS_RUNNING,
		RuntimeDetails: &triggers.Trigger_LadderState{LadderState: &triggers.LadderDetails{LadderLevels: 3}},
	}
}

func (unfilledTriggersHandler) GetTrigger(context.Context, *connect.Request[triggers.GetTriggerRequest]) (*connect.Response[triggers.GetTriggerResponse], error) {
	return connect.NewResponse(&triggers.GetTriggerResponse{Trigger: unfilledTwap()}), nil
}

func (unfilledTriggersHandler) ListTriggers(context.Context, *connect.Request[triggers.ListTriggersRequest]) (*connect.Response[triggers.ListTriggersResponse], error) {
	return connect.NewResponse(&triggers.ListTriggersResponse{Triggers: []*triggers.Trigger{unfilledTwap(), unfilledLadder()}}), nil
}

// Triggers.Get and Triggers.List read an unfilled TWAP's and ladder's
// executed quantity as a zero at the pair's scale, so it formats like Qty.
func TestTriggerReadsFormatUnfilledExecutedQty(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			mux := http.NewServeMux()
			path, handler := triggersv1connect.NewTriggersServiceHandler(unfilledTriggersHandler{})
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
			if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)}}}); err != nil {
				t.Fatal(err)
			}
			svc := NewTriggersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
			ctx := context.Background()
			got, err := svc.Get(ctx, nil, codecs.FormatID(9), nil)
			if err != nil {
				t.Fatal(err)
			}
			list, err := svc.List(ctx, nil, nil, nil, nil, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			for label, trigger := range map[string]struct {
				qty, executed func() (string, error)
			}{
				"Get twap":    {got.Qty.Format, got.Details.ExecutedQty.Format},
				"List twap":   {list.Triggers[0].Qty.Format, list.Triggers[0].Details.ExecutedQty.Format},
				"List ladder": {list.Triggers[1].Qty.Format, list.Triggers[1].Details.ExecutedQty.Format},
			} {
				if q, err := trigger.qty(); err != nil || q != "0.0006" {
					t.Errorf("%s: Qty.Format() = %q, %v", label, q, err)
				}
				if e, err := trigger.executed(); err != nil || e != "0" {
					t.Errorf("%s: Details.ExecutedQty.Format() = %q, %v; want \"0\"", label, e, err)
				}
			}
		})
	}
}
