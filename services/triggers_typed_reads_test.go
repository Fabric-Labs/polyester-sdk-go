package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggers "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// typedReadHandler answers GetTrigger with the fixture of the requested ID
// and ListTriggers with every fixture, recording each list request.
type typedReadHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	fixtures []*triggers.Trigger
	mu       sync.Mutex
	lists    []*triggers.ListTriggersRequest
}

func (h *typedReadHandler) GetTrigger(_ context.Context, req *connect.Request[triggers.GetTriggerRequest]) (*connect.Response[triggers.GetTriggerResponse], error) {
	for _, fixture := range h.fixtures {
		if fixture.GetTriggerId() == req.Msg.GetTriggerId() {
			return connect.NewResponse(&triggers.GetTriggerResponse{Trigger: fixture}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, errors.New("no such trigger"))
}

func (h *typedReadHandler) ListTriggers(_ context.Context, req *connect.Request[triggers.ListTriggersRequest]) (*connect.Response[triggers.ListTriggersResponse], error) {
	h.mu.Lock()
	h.lists = append(h.lists, req.Msg)
	h.mu.Unlock()
	return connect.NewResponse(&triggers.ListTriggersResponse{Triggers: h.fixtures}), nil
}

func (h *typedReadHandler) listRequests() []*triggers.ListTriggersRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*triggers.ListTriggersRequest(nil), h.lists...)
}

// newTypedReadService serves handler over the wire format ("binary" or
// "json"), with BTC-USDT (symbol_id 1) in the catalogs when loaded.
func newTypedReadService(t *testing.T, handler *typedReadHandler, wire string, loaded bool) *TriggersService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := triggersv1connect.NewTriggersServiceHandler(handler)
	mux.Handle(path, h)
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
	if loaded {
		if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
			"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8),
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), cats, nil, nil, nil, nil, nil)
}

// typedReadCase is a trigger fixture and the top-level cap its typed read
// must report: ticks (0 = none) and bps (0 = none).
type typedReadCase struct {
	name     string
	trigger  *triggers.Trigger
	ticks    int64
	bps      int32
	trailing *triggers.TrailingDetails
}

func twapMarketFixture(id uint64, cap *triggers.TwapMarketIoc) *triggers.Trigger {
	return &triggers.Trigger{
		TriggerId: id, SymbolId: 1, QtyScaled: 1000, Status: triggers.TriggerStatus_STATUS_CANCELED,
		Configuration: &triggers.Trigger_Twap{Twap: &triggers.TwapTrigger{
			Side: orders.Side_BUY, DurationMs: 60_000, SliceIntervalMs: 10_000,
			Execution: &triggers.TwapTrigger_MarketIoc{MarketIoc: cap},
		}},
		RuntimeDetails: &triggers.Trigger_TwapState{TwapState: &triggers.TwapDetails{TwapDurationMs: 60_000, TwapSliceIntervalMs: 10_000, SliceCount: 6}},
	}
}

func conditionalMarketFixture(id uint64, takeProfit bool, bps int32) *triggers.Trigger {
	cond := &triggers.ConditionalTrigger{
		TriggerPriceTicks: 50_000_000_000_000, Side: orders.Side_SELL,
		Child: &triggers.ConditionalChildExecution{Execution: &triggers.ConditionalChildExecution_MarketIoc{
			MarketIoc: &triggers.TriggerMarketIoc{MaxSlippage: &triggers.TriggerMarketIoc_MaxSlippageBps{MaxSlippageBps: bps}},
		}},
	}
	msg := &triggers.Trigger{TriggerId: id, SymbolId: 1, QtyScaled: 1000, Configuration: &triggers.Trigger_StopLoss{StopLoss: cond}}
	if takeProfit {
		msg.Configuration = &triggers.Trigger_TakeProfit{TakeProfit: cond}
	}
	return msg
}

func trailingFixture(id uint64, cap *triggers.TrailingStopTrigger, details *triggers.TrailingDetails) *triggers.Trigger {
	msg := &triggers.Trigger{TriggerId: id, SymbolId: 1, QtyScaled: 1000}
	if cap != nil {
		cap.Side = orders.Side_SELL
		cap.TrailingDistance = &triggers.TrailingStopTrigger_TrailingDistanceBps{TrailingDistanceBps: 200}
		msg.Configuration = &triggers.Trigger_TrailingStop{TrailingStop: cap}
	}
	if details != nil {
		msg.RuntimeDetails = &triggers.Trigger_Trailing{Trailing: details}
	}
	return msg
}

func requireTopLevelCap(t *testing.T, label string, got models.Trigger, ticks int64, bps int32) {
	t.Helper()
	if got.MaxSlippage.Ticks() != ticks || got.MaxSlippageBps != bps {
		t.Errorf("%s: MaxSlippage=%d ticks MaxSlippageBps=%d, want %d ticks %d bps", label, got.MaxSlippage.Ticks(), got.MaxSlippageBps, ticks, bps)
	}
}

// requireTypedReads reads every case with Get, List and the trigger stream
// decoder over both wire formats, and checks each with check.
func requireTypedReads(t *testing.T, cases []typedReadCase, loaded bool, check func(t *testing.T, label string, c typedReadCase, got models.Trigger)) {
	t.Helper()
	fixtures := make([]*triggers.Trigger, 0, len(cases))
	for _, c := range cases {
		fixtures = append(fixtures, c.trigger)
	}
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			svc := newTypedReadService(t, &typedReadHandler{fixtures: fixtures}, wire, loaded)
			ctx := context.Background()
			list, err := svc.List(ctx, nil, nil, nil, nil, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Triggers) != len(cases) {
				t.Fatalf("List returned %d triggers, want %d", len(list.Triggers), len(cases))
			}
			for i, c := range cases {
				got, err := svc.Get(ctx, nil, codecs.FormatUint64ID(c.trigger.GetTriggerId()), nil)
				if err != nil {
					t.Fatal(err)
				}
				check(t, wire+" Get "+c.name, c, *got)
				check(t, wire+" List "+c.name, c, list.Triggers[i])
				payload, err := proto.Marshal(c.trigger)
				if err != nil {
					t.Fatal(err)
				}
				streamed, err := svc.decodeStreamTrigger(payload)
				if err != nil {
					t.Fatal(err)
				}
				check(t, wire+" stream "+c.name, c, streamed)
			}
		})
	}
}

// POLY-9396: a market TWAP's per-slice cap reads back on the typed trigger
// in its exact variant, and an unset cap reads as none (the pair default).
// Stop-loss and take-profit market children are unchanged controls.
func TestPoly9396TwapMarketSliceCapOnTypedReads(t *testing.T) {
	cases := []typedReadCase{
		{name: "twap 10bps", trigger: twapMarketFixture(1, &triggers.TwapMarketIoc{MaxSlippage: &triggers.TwapMarketIoc_MaxSlippageBps{MaxSlippageBps: 10}}), bps: 10},
		{name: "twap 50bps", trigger: twapMarketFixture(2, &triggers.TwapMarketIoc{MaxSlippage: &triggers.TwapMarketIoc_MaxSlippageBps{MaxSlippageBps: 50}}), bps: 50},
		{name: "twap ticks", trigger: twapMarketFixture(3, &triggers.TwapMarketIoc{MaxSlippage: &triggers.TwapMarketIoc_MaxSlippageTicks{MaxSlippageTicks: 25_000_000_000}}), ticks: 25_000_000_000},
		{name: "twap unset", trigger: twapMarketFixture(4, &triggers.TwapMarketIoc{})},
		{name: "stop_loss 50bps", trigger: conditionalMarketFixture(5, false, 50), bps: 50},
		{name: "take_profit 50bps", trigger: conditionalMarketFixture(6, true, 50), bps: 50},
	}
	requireTypedReads(t, cases, true, func(t *testing.T, label string, c typedReadCase, got models.Trigger) {
		t.Helper()
		requireTopLevelCap(t, label, got, c.ticks, c.bps)
	})
}

// POLY-9425: a trailing stop's cap reads at the top level, as a stop-loss's
// does, in bps and in ticks; Details.trailing is unchanged. Without the
// configuration, the runtime details carry it.
func TestPoly9425TrailingStopCapAtTopLevel(t *testing.T) {
	bpsDetails := &triggers.TrailingDetails{TrailingDistanceBps: 200, MaxSlippageBps: 100}
	ticksDetails := &triggers.TrailingDetails{TrailingDistanceBps: 200, MaxSlippageTicks: 25_000_000_000}
	cases := []typedReadCase{
		{name: "trailing 100bps", trigger: trailingFixture(1, &triggers.TrailingStopTrigger{MaxSlippage: &triggers.TrailingStopTrigger_MaxSlippageBps{MaxSlippageBps: 100}}, bpsDetails), bps: 100, trailing: bpsDetails},
		{name: "trailing ticks", trigger: trailingFixture(2, &triggers.TrailingStopTrigger{MaxSlippage: &triggers.TrailingStopTrigger_MaxSlippageTicks{MaxSlippageTicks: 25_000_000_000}}, ticksDetails), ticks: 25_000_000_000, trailing: ticksDetails},
		{name: "trailing unset", trigger: trailingFixture(3, &triggers.TrailingStopTrigger{}, &triggers.TrailingDetails{TrailingDistanceBps: 200}), trailing: &triggers.TrailingDetails{TrailingDistanceBps: 200}},
		{name: "trailing runtime only", trigger: trailingFixture(4, nil, bpsDetails), bps: 100, trailing: bpsDetails},
		{name: "stop_loss 50bps", trigger: conditionalMarketFixture(5, false, 50), bps: 50},
	}
	requireTypedReads(t, cases, true, func(t *testing.T, label string, c typedReadCase, got models.Trigger) {
		t.Helper()
		requireTopLevelCap(t, label, got, c.ticks, c.bps)
		if c.trailing == nil {
			return
		}
		if got.Details == nil || got.Details.Case != "trailing" {
			t.Fatalf("%s: details %+v, want trailing", label, got.Details)
		}
		if got.Details.MaxSlippageBps != c.trailing.GetMaxSlippageBps() || got.Details.MaxSlippage.Ticks() != c.trailing.GetMaxSlippageTicks() {
			t.Errorf("%s: details cap %d ticks %d bps, want %d ticks %d bps", label, got.Details.MaxSlippage.Ticks(), got.Details.MaxSlippageBps, c.trailing.GetMaxSlippageTicks(), c.trailing.GetMaxSlippageBps())
		}
	})
}

// POLY-9424: with the catalogs loaded, trigger reads and the trigger stream
// name the pair; without them Symbol stays empty, as before.
func TestPoly9424TriggerReadsNameThePair(t *testing.T) {
	cases := []typedReadCase{
		{name: "stop_loss", trigger: conditionalMarketFixture(1, false, 50)},
		{name: "twap", trigger: twapMarketFixture(2, &triggers.TwapMarketIoc{})},
	}
	for _, loaded := range []bool{true, false} {
		want := "BTC-USDT"
		name := "catalogs"
		if !loaded {
			want, name = "", "no-catalogs"
		}
		t.Run(name, func(t *testing.T) {
			requireTypedReads(t, cases, loaded, func(t *testing.T, label string, _ typedReadCase, got models.Trigger) {
				t.Helper()
				if got.Symbol != want || got.SymbolID != 1 {
					t.Errorf("%s: Symbol=%q SymbolID=%d, want %q and 1", label, got.Symbol, got.SymbolID, want)
				}
			})
		})
	}
}

// POLY-9433: List sends the trigger type and parent order filters of
// ListTriggers, refuses invalid ones before sending, and sends neither when
// they are not given.
func TestPoly9433ListTriggerTypeAndParentOrderFilters(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		t.Run(wire, func(t *testing.T) {
			handler := &typedReadHandler{}
			svc := newTypedReadService(t, handler, wire, true)
			ctx := context.Background()
			parent := "12345"
			if _, err := svc.List(ctx, nil, nil, nil, nil, 10, nil, models.ListTriggersOptions{TriggerType: "ladder", ParentOrderID: &parent}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.List(ctx, nil, nil, nil, nil, 10, nil, models.ListTriggersOptions{TriggerType: "Take-Profit"}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.List(ctx, nil, nil, nil, nil, 10, nil); err != nil {
				t.Fatal(err)
			}
			sent := handler.listRequests()
			if len(sent) != 3 {
				t.Fatalf("sent %d list requests, want 3", len(sent))
			}
			if sent[0].GetTriggerType() != triggers.TriggerType_LADDER || sent[0].ParentOrderId == nil || sent[0].GetParentOrderId() != 12345 {
				t.Errorf("filtered list sent trigger_type=%v parent_order_id=%v", sent[0].GetTriggerType(), sent[0].ParentOrderId)
			}
			if sent[1].GetTriggerType() != triggers.TriggerType_TAKE_PROFIT || sent[1].ParentOrderId != nil {
				t.Errorf("type-only list sent trigger_type=%v parent_order_id=%v", sent[1].GetTriggerType(), sent[1].ParentOrderId)
			}
			if sent[2].GetTriggerType() != triggers.TriggerType_TRIGGER_TYPE_UNSPECIFIED || sent[2].ParentOrderId != nil {
				t.Errorf("unfiltered list sent trigger_type=%v parent_order_id=%v", sent[2].GetTriggerType(), sent[2].ParentOrderId)
			}

			bad := "not-an-id!"
			zero := "0"
			for _, tc := range []struct {
				name     string
				opts     []models.ListTriggersOptions
				contains string
			}{
				{"unknown type", []models.ListTriggersOptions{{TriggerType: "oco"}}, "trigger_type"},
				{"invalid parent", []models.ListTriggersOptions{{ParentOrderID: &bad}}, "parent_order_id"},
				{"zero parent", []models.ListTriggersOptions{{ParentOrderID: &zero}}, "parent_order_id"},
				{"two options", []models.ListTriggersOptions{{}, {}}, "at most one"},
			} {
				_, err := svc.List(ctx, nil, nil, nil, nil, 10, nil, tc.opts...)
				var validation *sdkerrors.ValidationError
				if !errors.As(err, &validation) || !strings.Contains(validation.Msg, tc.contains) {
					t.Errorf("%s: want ValidationError naming %q, got %v", tc.name, tc.contains, err)
				}
			}
			if got := len(handler.listRequests()); got != 3 {
				t.Fatalf("refused filters sent %d more requests", got-3)
			}
		})
	}
}
