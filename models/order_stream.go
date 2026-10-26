package models

// OrderEventKind names what happened to an order on the private order stream.
type OrderEventKind string

const (
	OrderEventAccepted     OrderEventKind = "accepted"
	OrderEventFilled       OrderEventKind = "filled"
	OrderEventCanceled     OrderEventKind = "canceled"
	OrderEventRejected     OrderEventKind = "rejected"
	OrderEventReplaced     OrderEventKind = "replaced"
	OrderEventAttachedRisk OrderEventKind = "attached_risk"
)

// OrderEvent is something that happened to an order, delivered in a unit of
// the private order stream.
type OrderEvent struct {
	OrderID string `json:"order_id"`
	// Version is the order's version after the event.
	Version uint32         `json:"version"`
	Kind    OrderEventKind `json:"kind"`
	// ReasonCode is the order's terminal reason code for canceled, rejected
	// and replaced events; 0 otherwise.
	ReasonCode uint32 `json:"reason_code,omitempty"`
	// ReplacementOrderID is the successor's public ID for replaced events.
	ReplacementOrderID string `json:"replacement_order_id,omitempty"`
	// Fill is set for filled events.
	Fill *OrderFill `json:"fill,omitempty"`
}

// OrderFill is one fill of an order.
type OrderFill struct {
	// MatchID equals the MatchID of the user trade for the same fill.
	MatchID string `json:"match_id"`
	// Ordinal is the fill's position among the order's fills, from 1.
	Ordinal uint32 `json:"ordinal"`
	// Role is "maker" or "taker".
	Role  string     `json:"role"`
	Qty   QtyScaled  `json:"qty"`
	Price PriceTicks `json:"price"`
}

// OrderCommandOutcomeKind names how a command was answered without changing
// any order.
type OrderCommandOutcomeKind string

const (
	OrderOutcomeAmendRejected   OrderCommandOutcomeKind = "amend_rejected"
	OrderOutcomeCancelRejected  OrderCommandOutcomeKind = "cancel_rejected"
	OrderOutcomeReplaceRejected OrderCommandOutcomeKind = "replace_rejected"
	OrderOutcomeRejected        OrderCommandOutcomeKind = "rejected"
	OrderOutcomeCancelAllDone   OrderCommandOutcomeKind = "cancel_all_done"
)

// OrderCommandOutcome answers an order command that changed no order.
type OrderCommandOutcome struct {
	Kind OrderCommandOutcomeKind `json:"kind"`
	// OrderID is the command's order; empty for a cancel by criteria.
	OrderID string `json:"order_id,omitempty"`
	// NewOrderID is a replace's successor; empty otherwise.
	NewOrderID    string `json:"new_order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	// Reason is the refusal's order error code, as the order write API names
	// the same refusal (for example "INSUFFICIENT_FUNDS"). Empty for
	// cancel_all_done.
	Reason string `json:"reason,omitempty"`
}

// OrderUnitEvent is one cause of a change in a unit of the private order
// stream. Exactly one of Order and Outcome is set.
type OrderUnitEvent struct {
	Order   *OrderEvent          `json:"order,omitempty"`
	Outcome *OrderCommandOutcome `json:"outcome,omitempty"`
}
