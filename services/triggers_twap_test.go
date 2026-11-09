package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// twapHandler answers trigger creates and event reads, and counts creates.
type twapHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	mu      sync.Mutex
	creates []*triggersv1.CreateTriggerRequest
	events  []*triggersv1.TriggerEvent
}

func (h *twapHandler) CreateTrigger(_ context.Context, req *connect.Request[triggersv1.CreateTriggerRequest]) (*connect.Response[triggersv1.CreateTriggerResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.creates = append(h.creates, req.Msg)
	return connect.NewResponse(&triggersv1.CreateTriggerResponse{TriggerId: 9}), nil
}

func (h *twapHandler) ListTriggerEvents(context.Context, *connect.Request[triggersv1.ListTriggerEventsRequest]) (*connect.Response[triggersv1.ListTriggerEventsResponse], error) {
	return connect.NewResponse(&triggersv1.ListTriggerEventsResponse{Events: h.events}), nil
}

func newTwapService(t *testing.T, h *twapHandler, wire string) *TriggersService {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := triggersv1connect.NewTriggersServiceHandler(h)
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
		"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

// scheduledSlices are two fired TWAP slices: the server sends no fire price
// for a scheduled slice, and a positive one when a price caused the fire.
func scheduledSlices() []*triggersv1.TriggerEvent {
	return []*triggersv1.TriggerEvent{
		{TriggerId: 9, TriggerType: triggersv1.TriggerType_TWAP, EventType: triggersv1.TriggerEventType_EVENT_FIRED, ChildSeq: 1, ChildOrderId: 101},
		{TriggerId: 9, TriggerType: triggersv1.TriggerType_TWAP, EventType: triggersv1.TriggerEventType_EVENT_FIRED, ChildSeq: 2, ChildOrderId: 102, FirePriceTicks: proto.Int64(123)},
		// Presence control only: the contract sends a positive price or none.
		{TriggerId: 9, TriggerType: triggersv1.TriggerType_TWAP, EventType: triggersv1.TriggerEventType_EVENT_FIRED, ChildSeq: 3, ChildOrderId: 103, FirePriceTicks: proto.Int64(0)},
	}
}

func checkScheduledSlices(t *testing.T, events []models.TriggerEvent) {
	t.Helper()
	if len(events) != 3 || events[0].ChildOrderID != codecs.FormatID(101) || events[1].ChildOrderID != codecs.FormatID(102) {
		t.Fatalf("events=%+v", events)
	}
	if fields := publicFields(t, events[0]); fields["fire_price"] != nil {
		t.Errorf("a slice without a fire price reads fire_price=%v", fields["fire_price"])
	}
	if fields := publicFields(t, events[1]); fmt.Sprint(fields["fire_price"]) != "map[ticks:123]" {
		t.Errorf("a positive fire price reads fire_price=%v", fields["fire_price"])
	}
	if events[2].FirePrice == nil || events[2].FirePrice.Ticks() != 0 {
		t.Errorf("an explicit zero fire price reads %v, want a present zero", events[2].FirePrice)
	}
}

// POLY-8959: a scheduled TWAP slice the server sends without a fire price
// keeps it absent through ListEvents, instead of reading a zero price.
func TestListEventsKeepsAnAbsentFirePriceAbsent(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			service := newTwapService(t, &twapHandler{events: scheduledSlices()}, wire)
			list, err := service.ListEvents(context.Background(), nil, "9", nil, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			checkScheduledSlices(t, list.Events)
		})
	}
}

// POLY-8959: the realtime trigger event decoder keeps the same absence.
func TestRealtimeTriggerEventsKeepAnAbsentFirePriceAbsent(t *testing.T) {
	payload, err := proto.Marshal(&triggersv1.TriggerEventBatch{Events: scheduledSlices()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := decode.TriggerEventBatchFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	checkScheduledSlices(t, events)
}

// POLY-8961: a market TWAP slice takes 1 to 10000 bps to buy and 1 to 9999 to
// sell, since a 100% cap leaves a sell no positive price. A refused cap sends
// nothing.
func TestTwapMarketSlippageCapsBySide(t *testing.T) {
	cases := []struct {
		side    string
		bps     int32
		refusal string // empty when the cap is sent
	}{
		{"buy", 50, ""}, {"buy", 9999, ""}, {"buy", 10000, ""}, {"buy", 10001, "between 1 and"},
		{"sell", 50, ""}, {"sell", 9999, ""}, {"sell", 10000, "between 1 and 9999"}, {"sell", 10001, "between 1 and"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d", tc.side, tc.bps), func(t *testing.T) {
			h := &twapHandler{}
			service := newTwapService(t, h, "binary")
			opts := baseTriggerOptions("twap")
			opts.MaxSlippageBps = optionPtr(tc.bps)
			_, err := service.Create(context.Background(), nil, "BTC-USDT", "twap", nil, tc.side, models.QtyFromDecimal("0.00036"), "market", nil, "", "", nil, nil, false, opts)
			h.mu.Lock()
			defer h.mu.Unlock()
			if tc.refusal == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(h.creates) != 1 || h.creates[0].GetTrigger().GetTwap().GetMarketIoc().GetMaxSlippageBps() != tc.bps {
					t.Fatalf("creates=%v", h.creates)
				}
				return
			}
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("want a ValidationError with %q, got %v", tc.refusal, err)
			}
			if len(h.creates) != 0 {
				t.Fatalf("a refused cap sent %d requests", len(h.creates))
			}
		})
	}
}
