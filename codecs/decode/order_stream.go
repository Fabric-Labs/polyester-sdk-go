package decode

import (
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// OrdersFromProto decodes orders with the same decoder as the order reads.
func OrdersFromProto(msgs []*orderv1.Order) []models.Order {
	out := make([]models.Order, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, OrderFromProto(msg))
	}
	return out
}

// OrderUnitEventsFromProto decodes a private order stream unit's events, in
// order. orders are the unit's changed orders; a fill takes its quantity's
// symbol from its order there. Events of a kind this SDK does not know are
// skipped.
func OrderUnitEventsFromProto(events []*privatewsv1.UnitEvent, orders []*orderv1.Order) []models.OrderUnitEvent {
	out := make([]models.OrderUnitEvent, 0, len(events))
	for _, event := range events {
		switch kind := event.GetKind().(type) {
		case *privatewsv1.UnitEvent_Order:
			out = append(out, models.OrderUnitEvent{Order: orderEventFromProto(kind.Order, orders)})
		case *privatewsv1.UnitEvent_Outcome:
			out = append(out, models.OrderUnitEvent{Outcome: commandOutcomeFromProto(kind.Outcome)})
		}
	}
	return out
}

func orderEventFromProto(msg *privatewsv1.OrderEvent, orders []*orderv1.Order) *models.OrderEvent {
	event := &models.OrderEvent{
		OrderID:    codecs.FormatUint64ID(msg.GetOrderId()),
		Version:    msg.GetVersion(),
		Kind:       orderEventKind(msg.GetKind()),
		ReasonCode: msg.GetReasonCode(),
	}
	if id := msg.GetReplacementOrderId(); id != 0 {
		event.ReplacementOrderID = codecs.FormatUint64ID(id)
	}
	if fill := msg.GetFill(); fill != nil {
		var symbolID *uint32
		for _, order := range orders {
			if order.GetOrderId() == msg.GetOrderId() {
				id := order.GetSymbolId()
				symbolID = &id
				break
			}
		}
		event.Fill = &models.OrderFill{
			MatchID: strconv.FormatUint(fill.GetMatchId(), 10),
			Ordinal: fill.GetOrdinal(),
			Role:    fillRole(fill.GetRole()),
			Qty:     codecs.DecodeQtyScaled(fill.GetQtyScaled(), -1, "", symbolID),
			Price:   codecs.DecodePriceTicks(fill.GetPriceTicks(), ""),
		}
	}
	return event
}

func commandOutcomeFromProto(msg *privatewsv1.CommandOutcome) *models.OrderCommandOutcome {
	outcome := &models.OrderCommandOutcome{
		Kind:          commandOutcomeKind(msg.GetKind()),
		ClientOrderID: msg.GetClientOrderId(),
	}
	if id := msg.GetOrderId(); id != 0 {
		outcome.OrderID = codecs.FormatUint64ID(id)
	}
	if id := msg.GetNewOrderId(); id != 0 {
		outcome.NewOrderID = codecs.FormatUint64ID(id)
	}
	if msg.GetReason() != orderv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		outcome.Reason = errorCodeName(msg.GetReason())
	}
	return outcome
}

func orderEventKind(kind privatewsv1.OrderEvent_Kind) models.OrderEventKind {
	switch kind {
	case privatewsv1.OrderEvent_ACCEPTED:
		return models.OrderEventAccepted
	case privatewsv1.OrderEvent_FILLED:
		return models.OrderEventFilled
	case privatewsv1.OrderEvent_CANCELED:
		return models.OrderEventCanceled
	case privatewsv1.OrderEvent_REJECTED:
		return models.OrderEventRejected
	case privatewsv1.OrderEvent_REPLACED:
		return models.OrderEventReplaced
	case privatewsv1.OrderEvent_ATTACHED_RISK:
		return models.OrderEventAttachedRisk
	default:
		return models.OrderEventKind(fmt.Sprintf("unknown(%d)", kind))
	}
}

func commandOutcomeKind(kind privatewsv1.CommandOutcome_Kind) models.OrderCommandOutcomeKind {
	switch kind {
	case privatewsv1.CommandOutcome_AMEND_REJECTED:
		return models.OrderOutcomeAmendRejected
	case privatewsv1.CommandOutcome_CANCEL_REJECTED:
		return models.OrderOutcomeCancelRejected
	case privatewsv1.CommandOutcome_REPLACE_REJECTED:
		return models.OrderOutcomeReplaceRejected
	case privatewsv1.CommandOutcome_REJECTED:
		return models.OrderOutcomeRejected
	case privatewsv1.CommandOutcome_CANCEL_ALL_DONE:
		return models.OrderOutcomeCancelAllDone
	default:
		return models.OrderCommandOutcomeKind(fmt.Sprintf("unknown(%d)", kind))
	}
}

func fillRole(role privatewsv1.Fill_Role) string {
	switch role {
	case privatewsv1.Fill_MAKER:
		return "maker"
	case privatewsv1.Fill_TAKER:
		return "taker"
	default:
		return ""
	}
}
