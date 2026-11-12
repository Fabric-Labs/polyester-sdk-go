package decode

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"google.golang.org/protobuf/proto"
)

// An iceberg order reads back its display and the hidden part of what is
// left, both at the pair's base scale; a plain order reads zero for both. A
// SELL sized by quote proceeds echoes its target.
func TestOrderReadsIcebergAndQuoteProceeds(t *testing.T) {
	order := OrderFromProto(&orderv1.Order{
		OrderId:                      7,
		SymbolId:                     1,
		OrigQtyScaled:                100_000_000,
		LeavesQtyScaled:              90_000_000,
		DisplayQtyScaled:             10_000_000,
		HiddenQtyScaled:              80_000_000,
		SubmittedQuoteProceedsScaled: proto.Int64(100_000_000),
		SelfTradePreventionMode:      orderv1.SelfTradePreventionMode_DECREMENT,
	})
	if order.DisplayQty.Scaled() != 10_000_000 || order.HiddenQty.Scaled() != 80_000_000 ||
		order.SubmittedQuoteProceedsScaled != "100000000" || order.SubmittedMaxQuoteDebitScaled != "" ||
		order.SelfTradePreventionMode != "decrement" {
		t.Fatalf("order=%+v", order)
	}
	scaled := ScaleOrder(order, func(uint32) (int, bool) { return 8, true })
	for name, qty := range map[string]interface {
		Format() (string, error)
	}{"display": scaled.DisplayQty, "hidden": scaled.HiddenQty} {
		if _, err := qty.Format(); err != nil {
			t.Fatalf("%s not scaled: %v", name, err)
		}
	}
	if text, _ := scaled.HiddenQty.Format(); text != "0.8" {
		t.Fatalf("hidden=%q", text)
	}
	plain := OrderFromProto(&orderv1.Order{OrderId: 8, SymbolId: 1, LeavesQtyScaled: 5})
	if plain.DisplayQty.Scaled() != 0 || plain.HiddenQty.Scaled() != 0 || plain.SubmittedQuoteProceedsScaled != "" {
		t.Fatalf("plain=%+v", plain)
	}
}

// The three new cancel reasons read by name, never as unknown.
func TestOrderReadsNewCancelReasons(t *testing.T) {
	for reason, want := range map[orderv1.CancelReason]string{
		orderv1.CancelReason_CANCEL_REASON_STP_DECREMENT:        "stp_decrement",
		orderv1.CancelReason_CANCEL_REASON_PRICE_BAND:           "price_band",
		orderv1.CancelReason_CANCEL_REASON_CANCEL_ON_DISCONNECT: "cancel_on_disconnect",
	} {
		order := OrderFromProto(&orderv1.Order{OrderId: 1, Status: orderv1.OrderStatus_CANCELED, CancelReason: reason})
		if order.CancelReason != want {
			t.Fatalf("%s read %q, want %q", reason, order.CancelReason, want)
		}
	}
}

// Create and batch-create answers echo a SELL's quote proceeds target.
func TestCreateResultsEchoQuoteProceeds(t *testing.T) {
	created, err := OrderMutationFromProto(&orderv1.CreateOrderResponse{
		OrderId: 9, ResolvedBaseQtyScaled: 163_000, SubmittedQuoteProceedsScaled: proto.Int64(100_000_000),
	})
	if err != nil || created.SubmittedQuoteProceedsScaled != "100000000" || created.ResolvedBaseQtyScaled != "163000" ||
		created.SubmittedMaxQuoteDebitScaled != "" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	batch, err := BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
		AcceptedCount: 2,
		Results: []*orderv1.BatchCreateResultItem{
			{Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{
				OrderId: 1, ResolvedBaseQtyScaled: 163_000, SubmittedQuoteProceedsScaled: proto.Int64(100_000_000),
			}}},
			{Outcome: &orderv1.BatchCreateResultItem_Accepted{Accepted: &orderv1.BatchCreateAccepted{
				OrderId: 2, SubmittedMaxQuoteDebitScaled: proto.Int64(50_000_000),
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, second := batch.Results[0], batch.Results[1]
	if first.SubmittedQuoteProceedsScaled != "100000000" || first.ResolvedBaseQtyScaled != "163000" ||
		second.SubmittedMaxQuoteDebitScaled != "50000000" || second.SubmittedQuoteProceedsScaled != "" || second.ResolvedBaseQtyScaled != "" {
		t.Fatalf("results=%+v", batch.Results)
	}
}

// A pair's price band reads from the spot config, with zero bounds kept, and
// through the catalogs' pair constraints. A pair without one has none.
func TestSpotConfigReadsPriceBand(t *testing.T) {
	cfg, err := SpotConfigFromProto(&marketdatav1.GetSpotConfigResponse{Pairs: []*marketdatav1.PairConfig{
		{SymbolId: 1, Symbol: "BTC-USDT", BaseQuantityScale: 8, QuoteQuantityScale: 6, TickSize: "0.01", StepSize: "0.00001",
			PriceBand: &marketdatav1.PriceBand{LowerBps: 500, UpperBps: 0, ReferenceWindowMs: 60_000}},
		{SymbolId: 2, Symbol: "ETH-USDT", BaseQuantityScale: 8, QuoteQuantityScale: 6, TickSize: "0.01", StepSize: "0.0001"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	band := cfg.Pairs[0].PriceBand
	if band == nil || band.LowerBps != 500 || band.UpperBps != 0 || band.ReferenceWindowMs != 60_000 || cfg.Pairs[1].PriceBand != nil {
		t.Fatalf("pairs=%+v", cfg.Pairs)
	}
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(cfg.Raw); err != nil {
		t.Fatal(err)
	}
	constraints, ok := cats.PairConstraintsForSymbol("BTC-USDT")
	if !ok || constraints.PriceBand == nil || *constraints.PriceBand != *band {
		t.Fatalf("constraints=%+v ok=%v", constraints, ok)
	}
	unbanded, ok := cats.PairConstraintsForSymbolID(2)
	if !ok || unbanded.PriceBand != nil {
		t.Fatalf("unbanded=%+v ok=%v", unbanded, ok)
	}
}

// Triggers read DECREMENT, a band refusal and the renamed quote-size failure.
func TestTriggerReadsDecrementAndNewFailureReasons(t *testing.T) {
	trigger := TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 3,
		Status:    triggersv1.TriggerStatus_STATUS_FAILED,
		TerminalReason: &triggersv1.Trigger_FailureReason{
			FailureReason: triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_PRICE_BAND_VIOLATION,
		},
		SelfTradePreventionMode: orderv1.SelfTradePreventionMode_DECREMENT,
	})
	if trigger.FailureReason != "price_band_violation" || trigger.SelfTradePreventionMode != "decrement" {
		t.Fatalf("trigger=%+v", trigger)
	}
	if got := TriggerFailureReasonLabel(triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_QUOTE_SIZE_TOO_SMALL); got != "quote_size_too_small" {
		t.Fatalf("quote size failure = %q", got)
	}
}

// A ladder level the book rejects is a fire_refused event naming the level,
// its price and why, after that level's fired event.
func TestLadderRejectedLevelIsFireRefused(t *testing.T) {
	reason := triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_POST_ONLY_CROSS
	events := TriggerEventsListFromProto(&triggersv1.ListTriggerEventsResponse{Events: []*triggersv1.TriggerEvent{
		{TriggerId: 4, TriggerType: triggersv1.TriggerType_LADDER, EventType: triggersv1.TriggerEventType_EVENT_FIRED, ChildSeq: 2, ChildOrderId: 40},
		{TriggerId: 4, TriggerType: triggersv1.TriggerType_LADDER, EventType: triggersv1.TriggerEventType_EVENT_FIRE_REFUSED, ChildSeq: 2,
			FireRefusalReason: &reason, FirePriceTicks: proto.Int64(60_000_000_000_000)},
	}}).Events
	refused := events[1]
	if events[0].EventType != "fired" || refused.EventType != "fire_refused" || refused.TriggerType != "ladder" ||
		refused.ChildSeq != events[0].ChildSeq || refused.FireRefusalReason != "post_only_cross" ||
		refused.FirePrice == nil || refused.FirePrice.Ticks() != 60_000_000_000_000 {
		t.Fatalf("events=%+v", events)
	}
}

// API-key policies read the two price-alert permissions by name.
func TestAPIPolicyReadsPriceAlertPermissions(t *testing.T) {
	policy := ApiPolicyMessageFromProto(&authv1.ApiPolicyView{Id: 1, Actions: []authv1.PolicyAction{
		authv1.PolicyAction_READ_PRICE_ALERTS, authv1.PolicyAction_MANAGE_PRICE_ALERTS,
	}})
	if len(policy.Actions) != 2 || policy.Actions[0] != "READ_PRICE_ALERTS" || policy.Actions[1] != "MANAGE_PRICE_ALERTS" {
		t.Fatalf("actions=%v", policy.Actions)
	}
	if authv1.PolicyAction_READ_PRICE_ALERTS != 36 || authv1.PolicyAction_MANAGE_PRICE_ALERTS != 37 {
		t.Fatal("price-alert permission values moved")
	}
}

// Changes and sections about the assistant read the assistant surface.
func TestChangelogReadsAssistantSurface(t *testing.T) {
	change := ChangelogChangeFromProto(&changelogv1.Change{Surfaces: []changelogv1.Surface{changelogv1.Surface_ASSISTANT}})
	if len(change.Surfaces) != 1 || change.Surfaces[0] != "assistant" {
		t.Fatalf("surfaces=%v", change.Surfaces)
	}
}

// A batch replace answers a value refusal of one item at that item, with its
// code and indexed violation, while the other items are judged on their own.
func TestBatchReplaceRejectsOneItemForItsPrice(t *testing.T) {
	got, err := BatchReplaceFromProto(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 5,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
		AcceptedCount:  1,
		RejectedCount:  1,
		Results: []*orderv1.BatchReplaceAdmissionItem{
			{ItemIndex: 0, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED, OldOrderId: 1, ClientOrderId: "a"},
			{ItemIndex: 1, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED, ClientOrderId: "b",
				Code: "PRICE_TICK_SIZE", Error: &orderv1.ErrorDetail{
					Code: orderv1.ErrorCode_ERROR_CODE_PRICE_TICK_SIZE,
					Violations: []*orderv1.FieldViolation{{
						FieldPath: "items[1].new_attached_risk.take_profit.trigger_price_ticks", RuleId: "PRICE_TICK_SIZE",
					}},
				}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rejected := got.Results[1]
	if got.Results[0].Status != "admitted" || rejected.Status != "rejected" || rejected.ItemIndex != 1 ||
		rejected.Code != "PRICE_TICK_SIZE" || len(rejected.Violations) != 1 ||
		rejected.Violations[0].FieldPath != "items[1].new_attached_risk.take_profit.trigger_price_ticks" {
		t.Fatalf("results=%+v", got.Results)
	}
}
