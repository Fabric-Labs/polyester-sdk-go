package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// Order end reasons read the same in every SDK: the short lowercase label,
// without the enum prefix. Order reads, order events and command outcomes
// spell a refusal the same way.
func TestOrderReasonsReadAsLowercaseLabels(t *testing.T) {
	order := mustDecoded(decode.OrderFromProto(&orderv1.Order{OrderId: 1, Status: orderv1.OrderStatus_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_PEG_LIMIT_VIOLATION}))
	if order.RejectReason != "peg_limit_violation" {
		t.Fatalf("order reject reason = %q, want peg_limit_violation", order.RejectReason)
	}
	order = mustDecoded(decode.OrderFromProto(&orderv1.Order{OrderId: 2, Status: orderv1.OrderStatus_CANCELED, CancelReason: orderv1.CancelReason_CANCEL_REASON_STP_DECREMENT}))
	if order.CancelReason != "stp_decrement" {
		t.Fatalf("order cancel reason = %q, want stp_decrement", order.CancelReason)
	}

	units := mustDecoded(decode.OrderUnitEventsFromProto([]*privatewsv1.UnitEvent{
		orderEvent(&privatewsv1.OrderEvent{OrderId: 3, Kind: privatewsv1.OrderEvent_REJECTED, RejectReason: orderv1.ErrorCode_ERROR_CODE_POST_ONLY_CROSS}),
		{Kind: &privatewsv1.UnitEvent_Outcome{Outcome: &privatewsv1.CommandOutcome{
			Kind: privatewsv1.CommandOutcome_AMEND_REJECTED, OrderId: 4, Reason: orderv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS,
		}}},
		{Kind: &privatewsv1.UnitEvent_Outcome{Outcome: &privatewsv1.CommandOutcome{
			Kind: privatewsv1.CommandOutcome_CANCEL_REJECTED, OrderId: 5, Reason: orderv1.ErrorCode(9999),
		}}},
	}, nil))
	if got := units[0].Order.RejectReason; got != "post_only_cross" {
		t.Fatalf("order event reject reason = %q, want post_only_cross", got)
	}
	if got := units[1].Outcome.Reason; got != "insufficient_funds" {
		t.Fatalf("command outcome reason = %q, want insufficient_funds", got)
	}
	// A code newer than the SDK stays readable as before.
	if got := units[2].Outcome.Reason; got != "UNKNOWN_ERROR_CODE(9999)" {
		t.Fatalf("unknown command outcome reason = %q", got)
	}

	// Trigger reasons already follow the rule.
	trigger := mustDecoded(decode.TriggerFromProto(&triggersv1.Trigger{TriggerId: 6, TerminalReason: &triggersv1.Trigger_FailureReason{
		FailureReason: triggersv1.TriggerFailureReason_TRIGGER_FAILURE_REASON_INSUFFICIENT_FUNDS,
	}}))
	if trigger.FailureReason != "insufficient_funds" {
		t.Fatalf("trigger failure reason = %q", trigger.FailureReason)
	}
}
