package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// When an attached exit fires while its order still rests, the order's rest
// is canceled with CANCEL_REASON_ATTACHED_EXIT_FIRED (18).
func TestAttachedExitFiredCancelReason(t *testing.T) {
	if orderv1.CancelReason_CANCEL_REASON_ATTACHED_EXIT_FIRED != 18 ||
		orderv1.ErrorCode_ERROR_CODE_SELF_TRADE_PREVENTED != 307 ||
		triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_SELF_TRADE_PREVENTED != 249 {
		t.Fatal("generated attached-exit reason numbers changed")
	}
	order := decode.OrderFromProto(&orderv1.Order{
		OrderId: 1, Status: orderv1.OrderStatus_CANCELED,
		CancelReason: orderv1.CancelReason_CANCEL_REASON_ATTACHED_EXIT_FIRED,
	})
	if order.CancelReason != "attached_exit_fired" {
		t.Fatalf("order cancel reason=%q", order.CancelReason)
	}
	events := decode.OrderUnitEventsFromProto([]*privatewsv1.UnitEvent{
		orderEvent(&privatewsv1.OrderEvent{OrderId: 1, Kind: privatewsv1.OrderEvent_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_ATTACHED_EXIT_FIRED}),
	}, nil)
	if len(events) != 1 || events[0].Order.CancelReason != "attached_exit_fired" {
		t.Fatalf("order stream events=%+v", events)
	}
	rejected := decode.OrderFromProto(&orderv1.Order{
		OrderId: 2, Status: orderv1.OrderStatus_REJECTED,
		RejectReason: orderv1.ErrorCode_ERROR_CODE_SELF_TRADE_PREVENTED,
	})
	if rejected.RejectReason != "SELF_TRADE_PREVENTED" {
		t.Fatalf("reject reason=%q", rejected.RejectReason)
	}
}

// Self-trade prevention expiring an attached exit's order is a fire refusal:
// the trigger stays live and the event's ChildSeq names the expired order.
func TestSelfTradePreventedIsAFireRefusal(t *testing.T) {
	reason := triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_SELF_TRADE_PREVENTED
	got := decode.TriggerEventMessageFromProto(&triggersv1.TriggerEvent{
		TriggerId: 11, EventType: triggersv1.TriggerEventType_EVENT_FIRE_REFUSED,
		ChildSeq: 2, FireRefusalReason: &reason,
	})
	if got.EventType != "fire_refused" || got.FireRefusalReason != "self_trade_prevented" || got.ChildSeq != 2 ||
		got.CancelReason != "" || got.FailureReason != "" {
		t.Fatalf("event=%+v", got)
	}
	trigger := decode.TriggerFromProto(&triggersv1.Trigger{TriggerId: 11, Status: triggersv1.TriggerStatus_STATUS_ARMED})
	if trigger.Status != "armed" || trigger.FailureReason != "" {
		t.Fatalf("trigger=%+v", trigger)
	}
}

func TestOrderReadsSelfTradePreventionMode(t *testing.T) {
	cases := map[orderv1.SelfTradePreventionMode]string{
		orderv1.SelfTradePreventionMode_SELF_TRADE_PREVENTION_MODE_UNSPECIFIED: "",
		orderv1.SelfTradePreventionMode_EXPIRE_MAKER:                           "expire_maker",
		orderv1.SelfTradePreventionMode_EXPIRE_TAKER:                           "expire_taker",
		orderv1.SelfTradePreventionMode_EXPIRE_BOTH:                            "expire_both",
		orderv1.SelfTradePreventionMode(9):                                     "unknown(9)",
	}
	for mode, want := range cases {
		got := decode.OrderFromProto(&orderv1.Order{OrderId: 1, SelfTradePreventionMode: mode})
		if got.SelfTradePreventionMode != want {
			t.Fatalf("mode %d: got %q want %q", mode, got.SelfTradePreventionMode, want)
		}
	}
}

func TestGetSubaccountDecodesBalancesOnlyWhenPresent(t *testing.T) {
	without := decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: 42}})
	if without.Balances != nil || without.Subaccount == nil || without.Subaccount.SubaccountID == "" {
		t.Fatalf("without balances=%+v", without)
	}
	with := decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{
		Subaccount: &authv1.Subaccount{Id: 42},
		Balances: &ledgerrdv1.GetBalancesResponse{Balances: []*ledgerrdv1.AssetBalance{{
			AssetId:         1,
			Trading:         &typev1.U128{Lo: 1_500_000_000_000_000_000},
			Available:       &typev1.U128{Lo: 1_500_000_000_000_000_000},
			TradingRevision: 3,
		}}},
	})
	if with.Balances == nil || len(with.Balances.Balances) != 1 {
		t.Fatalf("with balances=%+v", with.Balances)
	}
	b := with.Balances.Balances[0]
	if b.AssetID != 1 || b.Trading != "1500000000000000000" || b.Available != "1500000000000000000" ||
		b.Funding != "0" || b.Reserved != "0" || b.TradingRevision != 3 {
		t.Fatalf("balance=%+v", b)
	}
	empty := decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{
		Subaccount: &authv1.Subaccount{Id: 42}, Balances: &ledgerrdv1.GetBalancesResponse{},
	})
	if empty.Balances == nil || empty.Balances.Balances == nil || len(empty.Balances.Balances) != 0 {
		t.Fatalf("empty balances=%+v", empty.Balances)
	}
}
