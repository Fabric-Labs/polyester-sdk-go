package decode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var triggerStatusLabels = map[triggersv1.TriggerStatus]string{
	triggersv1.TriggerStatus_STATUS_CREATED:   "created",
	triggersv1.TriggerStatus_STATUS_ARMED:     "armed",
	triggersv1.TriggerStatus_STATUS_RUNNING:   "running",
	triggersv1.TriggerStatus_STATUS_COMPLETED: "completed",
	triggersv1.TriggerStatus_STATUS_CANCELED:  "cancelled",
	triggersv1.TriggerStatus_STATUS_FAILED:    "failed",
	triggersv1.TriggerStatus_STATUS_PAUSED:    "paused",
}

var triggerStatusFromLabel = map[string]triggersv1.TriggerStatus{
	"created":   triggersv1.TriggerStatus_STATUS_CREATED,
	"armed":     triggersv1.TriggerStatus_STATUS_ARMED,
	"running":   triggersv1.TriggerStatus_STATUS_RUNNING,
	"completed": triggersv1.TriggerStatus_STATUS_COMPLETED,
	"cancelled": triggersv1.TriggerStatus_STATUS_CANCELED,
	"canceled":  triggersv1.TriggerStatus_STATUS_CANCELED,
	"failed":    triggersv1.TriggerStatus_STATUS_FAILED,
	"paused":    triggersv1.TriggerStatus_STATUS_PAUSED,
}

// TriggerStatusLabel maps proto trigger status to SDK output labels.
func TriggerStatusLabel(status triggersv1.TriggerStatus) string {
	if label, ok := triggerStatusLabels[status]; ok {
		return label
	}
	if status == triggersv1.TriggerStatus_STATUS_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(status.String(), "STATUS_"))
}

// TriggerStatusFromLabel parses a trigger status filter label.
func TriggerStatusFromLabel(label string) (triggersv1.TriggerStatus, error) {
	key := strings.ToLower(strings.TrimSpace(label))
	if status, ok := triggerStatusFromLabel[key]; ok {
		return status, nil
	}
	return triggersv1.TriggerStatus_STATUS_UNSPECIFIED, fmt.Errorf(
		"invalid trigger status %q; expected one of: created, armed, running, completed, cancelled, failed, paused",
		label,
	)
}

var triggerEventTypeLabels = map[triggersv1.TriggerEventType]string{
	triggersv1.TriggerEventType_EVENT_FIRED:    "fired",
	triggersv1.TriggerEventType_EVENT_CANCELED: "canceled",
	triggersv1.TriggerEventType_EVENT_UPDATED:  "updated",
	triggersv1.TriggerEventType_EVENT_FAILED:   "failed",
}

var triggerEventTypeFromLabel = map[string]triggersv1.TriggerEventType{
	"fired":     triggersv1.TriggerEventType_EVENT_FIRED,
	"canceled":  triggersv1.TriggerEventType_EVENT_CANCELED,
	"cancelled": triggersv1.TriggerEventType_EVENT_CANCELED,
	"updated":   triggersv1.TriggerEventType_EVENT_UPDATED,
	"failed":    triggersv1.TriggerEventType_EVENT_FAILED,
}

// TriggerEventTypeLabel maps proto trigger event type to SDK output labels.
func TriggerEventTypeLabel(eventType triggersv1.TriggerEventType) string {
	if label, ok := triggerEventTypeLabels[eventType]; ok {
		return label
	}
	if eventType == triggersv1.TriggerEventType_EVENT_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(eventType.String(), "EVENT_"))
}

// TriggerEventTypeFromLabel parses a trigger event type filter label.
func TriggerEventTypeFromLabel(label string) (triggersv1.TriggerEventType, error) {
	key := strings.ToLower(strings.TrimSpace(label))
	if eventType, ok := triggerEventTypeFromLabel[key]; ok {
		return eventType, nil
	}
	return triggersv1.TriggerEventType_EVENT_UNSPECIFIED, fmt.Errorf(
		"invalid trigger event type %q; expected one of: fired, canceled, updated, failed",
		label,
	)
}

// TriggerCancelReasonLabel maps proto cancel reason to a friendly label.
// Unspecified (0) and unset decode as "".
func TriggerCancelReasonLabel(reason triggersv1.TriggerCancelReason) string {
	return codecs.ProtoEnumName(reason, reason.Number())
}

// TriggerFailureReasonLabel maps proto failure reason to a friendly label.
// Unspecified (0) and unset decode as "".
func TriggerFailureReasonLabel(reason triggersv1.TriggerFailureReason) string {
	return codecs.ProtoEnumName(reason, reason.Number())
}

// TriggerTypeLabel maps proto trigger type to SDK output labels.
func TriggerTypeLabel(triggerType triggersv1.TriggerType) string {
	if triggerType == triggersv1.TriggerType_TRIGGER_TYPE_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(triggerType.String())
}

func triggerMutationResult(triggerID uint64, status triggersv1.TriggerStatus) models.TriggerMutationResult {
	return models.TriggerMutationResult{
		TriggerID: codecs.FormatUint64ID(triggerID),
		Status:    TriggerStatusLabel(status),
	}
}

func triggerDirectionLabel(v orderv1.TriggerDirection) string {
	switch v {
	case orderv1.TriggerDirection_ABOVE:
		return "above"
	case orderv1.TriggerDirection_BELOW:
		return "below"
	default:
		return ""
	}
}

func feeAssetLabel(v orderv1.FeeAsset) string {
	switch v {
	case orderv1.FeeAsset_QUOTE:
		return "quote"
	case orderv1.FeeAsset_BASE:
		return "base"
	default:
		return ""
	}
}

func stpModeLabel(v orderv1.SelfTradePreventionMode) string {
	switch v {
	case orderv1.SelfTradePreventionMode_EXPIRE_TAKER:
		return "expire_taker"
	case orderv1.SelfTradePreventionMode_EXPIRE_MAKER:
		return "expire_maker"
	case orderv1.SelfTradePreventionMode_EXPIRE_BOTH:
		return "expire_both"
	default:
		return ""
	}
}

func ladderDistributionLabel(v triggersv1.LadderDistribution) string {
	switch v {
	case triggersv1.LadderDistribution_LINEAR:
		return "linear"
	case triggersv1.LadderDistribution_GEOMETRIC:
		return "geometric"
	case triggersv1.LadderDistribution_WEIGHTED_FAVORABLE:
		return "weighted_favorable"
	default:
		return ""
	}
}

func triggerDetailsFromProto(msg *triggersv1.Trigger) *models.TriggerDetails {
	if msg == nil {
		return nil
	}
	sid := msg.GetSymbolId()
	symbol := ""
	switch d := msg.GetRuntimeDetails().(type) {
	case *triggersv1.Trigger_Stop:
		stop := d.Stop
		if stop == nil {
			return nil
		}
		return &models.TriggerDetails{
			Case:               "stop",
			TriggerPrice:       codecs.DecodePriceTicks(stop.GetTriggerPriceTicks(), symbol),
			TriggerPriceSource: codecs.TriggerPriceSourceName(stop.GetTriggerPriceSource()),
			TriggerDirection:   triggerDirectionLabel(stop.GetTriggerDirection()),
		}
	case *triggersv1.Trigger_Trailing:
		tr := d.Trailing
		if tr == nil {
			return nil
		}
		out := &models.TriggerDetails{
			Case:                "trailing",
			TrailingDistanceBps: tr.GetTrailingDistanceBps(),
			MaxSlippageBps:      tr.GetMaxSlippageBps(),
			TriggerPriceSource:  codecs.TriggerPriceSourceName(tr.GetTriggerPriceSource()),
			TriggerDirection:    triggerDirectionLabel(tr.GetTriggerDirection()),
		}
		if tr.GetTrailingDistanceTicks() > 0 {
			out.TrailingDistance = codecs.DecodePriceTicks(tr.GetTrailingDistanceTicks(), symbol)
		}
		if tr.GetActivationPriceTicks() > 0 {
			out.ActivationPrice = codecs.DecodePriceTicks(tr.GetActivationPriceTicks(), symbol)
		}
		if tr.GetPeakPriceTicks() > 0 {
			out.PeakPrice = codecs.DecodePriceTicks(tr.GetPeakPriceTicks(), symbol)
		}
		if tr.GetTroughPriceTicks() > 0 {
			out.TroughPrice = codecs.DecodePriceTicks(tr.GetTroughPriceTicks(), symbol)
		}
		if tr.GetMaxSlippageTicks() > 0 {
			out.MaxSlippage = codecs.DecodePriceTicks(int64(tr.GetMaxSlippageTicks()), symbol)
		}
		return out
	case *triggersv1.Trigger_TwapState:
		twap := d.TwapState
		if twap == nil {
			return nil
		}
		out := &models.TriggerDetails{
			Case:                "twap",
			TwapDurationMs:      twap.GetTwapDurationMs(),
			TwapSliceIntervalMs: twap.GetTwapSliceIntervalMs(),
			SliceIdx:            twap.GetSliceIdx(),
			SliceCount:          twap.GetSliceCount(),
		}
		if twap.GetExecutedQtyScaled() != 0 {
			out.ExecutedQty = codecs.DecodeQtyScaled(twap.GetExecutedQtyScaled(), -1, symbol, &sid)
		}
		return out
	case *triggersv1.Trigger_LadderState:
		ladder := d.LadderState
		if ladder == nil {
			return nil
		}
		out := &models.TriggerDetails{
			Case:               "ladder",
			LadderLevels:       ladder.GetLadderLevels(),
			LadderDistribution: ladderDistributionLabel(ladder.GetLadderDistribution()),
		}
		if ladder.GetLadderPriceMinTicks() > 0 {
			out.LadderPriceMin = codecs.DecodePriceTicks(ladder.GetLadderPriceMinTicks(), symbol)
		}
		if ladder.GetLadderPriceMaxTicks() > 0 {
			out.LadderPriceMax = codecs.DecodePriceTicks(ladder.GetLadderPriceMaxTicks(), symbol)
		}
		return out
	default:
		return nil
	}
}

// triggerConfigProjection derives the flat public trigger fields
// (type/side/order_type/tif/post_only/limit_price/trigger_price) from the
// immutable Configuration oneof.
func triggerConfigProjection(msg *triggersv1.Trigger) (triggerType, side, orderType, tif string, postOnly bool, limitPrice, triggerPrice models.PriceTicks) {
	symbol := ""
	switch cfg := msg.GetConfiguration().(type) {
	case *triggersv1.Trigger_StopLoss:
		triggerType = "stop_loss"
		side, orderType, tif, postOnly, limitPrice = conditionalChildProjection(cfg.StopLoss, symbol)
		if cfg.StopLoss != nil {
			triggerPrice = codecs.DecodePriceTicks(cfg.StopLoss.GetTriggerPriceTicks(), symbol)
		}
	case *triggersv1.Trigger_TakeProfit:
		triggerType = "take_profit"
		side, orderType, tif, postOnly, limitPrice = conditionalChildProjection(cfg.TakeProfit, symbol)
		if cfg.TakeProfit != nil {
			triggerPrice = codecs.DecodePriceTicks(cfg.TakeProfit.GetTriggerPriceTicks(), symbol)
		}
	case *triggersv1.Trigger_TrailingStop:
		// Trailing stop fires a market-IOC child; side is on the wire config
		// (standalone historically SELL; attached may be either side).
		triggerType = "trailing_stop"
		orderType = "market"
		tif = "ioc"
		if t := cfg.TrailingStop; t != nil {
			side = codecs.OrderSideName(t.GetSide())
		}
	case *triggersv1.Trigger_Twap:
		triggerType = "twap"
		if t := cfg.Twap; t != nil {
			side = codecs.OrderSideName(t.GetSide())
			switch {
			case t.GetLimitGtc() != nil:
				orderType = "limit"
				tif = "gtc"
				limitPrice = codecs.DecodePriceTicks(t.GetLimitGtc().GetPriceTicks(), symbol)
			case t.GetMarketIoc() != nil:
				orderType = "market"
				tif = "ioc"
			}
		}
	case *triggersv1.Trigger_Ladder:
		triggerType = "ladder"
		if l := cfg.Ladder; l != nil {
			side = codecs.OrderSideName(l.GetSide())
			orderType = "limit"
			tif = "gtc"
			postOnly = l.GetPostOnly()
		}
	}
	return
}

// conditionalChildProjection derives flat child fields from a stop-loss /
// take-profit ConditionalTrigger configuration.
func conditionalChildProjection(cond *triggersv1.ConditionalTrigger, symbol string) (side, orderType, tif string, postOnly bool, limitPrice models.PriceTicks) {
	if cond == nil {
		return
	}
	side = codecs.OrderSideName(cond.GetSide())
	child := cond.GetChild()
	if child == nil {
		return
	}
	switch {
	case child.GetMarketIoc() != nil:
		orderType = "market"
		tif = "ioc"
	case child.GetLimitGtc() != nil:
		lg := child.GetLimitGtc()
		orderType = "limit"
		tif = "gtc"
		postOnly = lg.GetPostOnly()
		limitPrice = codecs.DecodePriceTicks(lg.GetPriceTicks(), symbol)
	case child.GetLimitIoc() != nil:
		orderType = "limit"
		tif = "ioc"
		limitPrice = codecs.DecodePriceTicks(child.GetLimitIoc().GetPriceTicks(), symbol)
	case child.GetLimitFok() != nil:
		orderType = "limit"
		tif = "fok"
		limitPrice = codecs.DecodePriceTicks(child.GetLimitFok().GetPriceTicks(), symbol)
	}
	return
}

// TriggerFromProto decodes a trigger message.
func TriggerFromProto(msg *triggersv1.Trigger) models.Trigger {
	if msg == nil {
		return models.Trigger{}
	}
	sid := msg.GetSymbolId()
	details := triggerDetailsFromProto(msg)
	triggerType, side, orderType, tif, postOnly, limitPrice, triggerPrice := triggerConfigProjection(msg)
	// Fall back to stop runtime details for the trigger price convenience field.
	if triggerPrice.Ticks() == 0 && details != nil && details.Case == "stop" {
		triggerPrice = details.TriggerPrice
	}
	var parentOrderID string
	if msg.ParentOrderId != nil {
		parentOrderID = codecs.FormatUint64ID(msg.GetParentOrderId())
	}
	cancelReason, failureReason := triggerReasonsFromTrigger(msg)
	return models.Trigger{
		TriggerID:               codecs.FormatUint64ID(msg.GetTriggerId()),
		SubaccountID:            codecs.FormatUint64ID(msg.GetSubaccountId()),
		SymbolID:                sid,
		Symbol:                  "",
		TriggerType:             triggerType,
		Status:                  TriggerStatusLabel(msg.GetStatus()),
		ParentOrderID:           parentOrderID,
		Side:                    side,
		OrderType:               orderType,
		TimeInForce:             tif,
		Qty:                     codecs.DecodeQtyScaled(msg.GetQtyScaled(), -1, "", &sid),
		FeeAsset:                feeAssetLabel(msg.GetFeeAsset()),
		SelfTradePreventionMode: stpModeLabel(msg.GetSelfTradePreventionMode()),
		PostOnly:                postOnly,
		LimitPrice:              limitPrice,
		TriggerPrice:            triggerPrice,
		ClientTriggerID:         msg.GetClientTriggerId(),
		CreatedAt:               timestampTime(msg.GetCreatedAt()),
		UpdatedAt:               timestampTime(msg.GetUpdatedAt()),
		ArmedAt:                 timestampTime(msg.GetArmedAt()),
		CompletedAt:             timestampTime(msg.GetCompletedAt()),
		CancelReason:            cancelReason,
		FailureReason:           failureReason,
		Details:                 details,
	}
}

// TriggersListFromProto decodes list triggers response.
func TriggersListFromProto(msg *triggersv1.ListTriggersResponse) models.TriggersList {
	out := make([]models.Trigger, 0, len(msg.GetTriggers()))
	for _, t := range msg.GetTriggers() {
		out = append(out, TriggerFromProto(t))
	}
	return models.TriggersList{
		Triggers:      out,
		Total:         len(out),
		NextPageToken: msg.GetNextPageToken(),
	}
}

// GetTriggerFromProto decodes get trigger response.
func GetTriggerFromProto(msg *triggersv1.GetTriggerResponse) *models.Trigger {
	if msg.GetTrigger() == nil {
		return nil
	}
	t := TriggerFromProto(msg.GetTrigger())
	return &t
}

// TriggerMutationFromProto decodes create trigger response.
//
// CreateTriggerResponse acknowledges admission only and no longer carries a
// status field; synthesize "accepted".
func TriggerMutationFromProto(msg *triggersv1.CreateTriggerResponse) models.TriggerMutationResult {
	return models.TriggerMutationResult{
		TriggerID: codecs.FormatUint64ID(msg.GetTriggerId()),
		Status:    "accepted",
	}
}

// TriggerMutationFromCancel decodes cancel trigger response.
func TriggerMutationFromCancel(msg *triggersv1.CancelTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

// TriggerMutationFromPause decodes pause trigger response.
func TriggerMutationFromPause(msg *triggersv1.PauseTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

// TriggerMutationFromResume decodes resume trigger response.
func TriggerMutationFromResume(msg *triggersv1.ResumeTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

// TriggerMutationFromModify decodes modify trigger response.
func TriggerMutationFromModify(msg *triggersv1.ModifyTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

// TriggerEventsListFromProto decodes list trigger events response.
func TriggerEventsListFromProto(msg *triggersv1.ListTriggerEventsResponse) models.TriggerEventsList {
	out := make([]models.TriggerEvent, 0, len(msg.GetEvents()))
	for _, e := range msg.GetEvents() {
		out = append(out, TriggerEventMessageFromProto(e))
	}
	return models.TriggerEventsList{
		Events:        out,
		NextPageToken: msg.GetNextPageToken(),
	}
}

// TriggerEventMessageFromProto decodes one trigger event.
func TriggerEventMessageFromProto(e *triggersv1.TriggerEvent) models.TriggerEvent {
	if e == nil {
		return models.TriggerEvent{}
	}
	tsNs := ""
	if e.GetTsNs() != 0 {
		tsNs = strconv.FormatUint(e.GetTsNs(), 10)
	}
	childOrderID := ""
	if e.GetChildOrderId() != 0 {
		childOrderID = codecs.FormatUint64ID(e.GetChildOrderId())
	}
	subaccountID := ""
	if e.GetSubaccountId() != 0 {
		subaccountID = codecs.FormatUint64ID(e.GetSubaccountId())
	}
	cancelReason, failureReason := triggerReasonsFromEvent(e)
	return models.TriggerEvent{
		TriggerID:     codecs.FormatUint64ID(e.GetTriggerId()),
		SubaccountID:  subaccountID,
		SymbolID:      e.GetSymbolId(),
		TriggerType:   TriggerTypeLabel(e.GetTriggerType()),
		EventType:     TriggerEventTypeLabel(e.GetEventType()),
		TsNs:          tsNs,
		ChildSeq:      e.GetChildSeq(),
		ChildOrderID:  childOrderID,
		FirePrice:     codecs.DecodePriceTicks(e.GetFirePriceTicks(), ""),
		CancelReason:  cancelReason,
		FailureReason: failureReason,
	}
}

func triggerReasonsFromTrigger(msg *triggersv1.Trigger) (string, string) {
	if msg == nil {
		return "", ""
	}
	switch msg.GetTerminalReason().(type) {
	case *triggersv1.Trigger_CancelReason:
		return TriggerCancelReasonLabel(msg.GetCancelReason()), ""
	case *triggersv1.Trigger_FailureReason:
		return "", TriggerFailureReasonLabel(msg.GetFailureReason())
	default:
		return "", ""
	}
}

func triggerReasonsFromEvent(msg *triggersv1.TriggerEvent) (string, string) {
	if msg == nil {
		return "", ""
	}
	switch msg.GetTerminalReason().(type) {
	case *triggersv1.TriggerEvent_CancelReason:
		return TriggerCancelReasonLabel(msg.GetCancelReason()), ""
	case *triggersv1.TriggerEvent_FailureReason:
		return "", TriggerFailureReasonLabel(msg.GetFailureReason())
	default:
		return "", ""
	}
}
