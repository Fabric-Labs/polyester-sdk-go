package decode

import (
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
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

func triggerMutationResult(triggerID uint64, status triggersv1.TriggerStatus) models.TriggerMutationResult {
	return models.TriggerMutationResult{
		TriggerID: codecs.FormatUint64ID(triggerID),
		Status:    TriggerStatusLabel(status),
	}
}

func triggerPriceTicks(msg *triggersv1.Trigger) int64 {
	if msg == nil {
		return 0
	}
	if stop := msg.GetStop(); stop != nil {
		return stop.GetTriggerPriceTicks()
	}
	return 0
}

func TriggerFromProto(msg *triggersv1.Trigger) models.Trigger {
	if msg == nil {
		return models.Trigger{}
	}
	return models.Trigger{
		TriggerID: codecs.FormatUint64ID(msg.GetTriggerId()), SymbolID: msg.GetSymbolId(), Symbol: msg.GetSymbol(),
		TriggerType: msg.GetTriggerType().String(), Status: TriggerStatusLabel(msg.GetStatus()), Side: msg.GetSide().String(),
		QtyScaled:         strconv.FormatInt(msg.GetQtyScaled(), 10),
		TriggerPriceTicks: strconv.FormatInt(triggerPriceTicks(msg), 10),
		ClientTriggerID:   msg.GetClientTriggerId(),
	}
}

func TriggersListFromProto(msg *triggersv1.ListTriggersResponse) models.TriggersList {
	out := make([]models.Trigger, 0, len(msg.GetTriggers()))
	for _, t := range msg.GetTriggers() {
		out = append(out, TriggerFromProto(t))
	}
	return models.TriggersList{Triggers: out, Total: len(out)}
}

func GetTriggerFromProto(msg *triggersv1.GetTriggerResponse) *models.Trigger {
	if msg.GetTrigger() == nil {
		return nil
	}
	t := TriggerFromProto(msg.GetTrigger())
	return &t
}

func TriggerMutationFromProto(msg *triggersv1.CreateTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

func TriggerMutationFromCancel(msg *triggersv1.CancelTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

func TriggerMutationFromPause(msg *triggersv1.PauseTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

func TriggerMutationFromResume(msg *triggersv1.ResumeTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

func TriggerMutationFromModify(msg *triggersv1.ModifyTriggerResponse) models.TriggerMutationResult {
	return triggerMutationResult(msg.GetTriggerId(), msg.GetStatus())
}

func TriggerEventsListFromProto(msg *triggersv1.ListTriggerEventsResponse) models.TriggerEventsList {
	out := make([]models.TriggerEvent, 0, len(msg.GetEvents()))
	for _, e := range msg.GetEvents() {
		out = append(out, TriggerEventMessageFromProto(e))
	}
	return models.TriggerEventsList{Events: out}
}

// TriggerEventMessageFromProto decodes one trigger event.
func TriggerEventMessageFromProto(e *triggersv1.TriggerEvent) models.TriggerEvent {
	if e == nil {
		return models.TriggerEvent{}
	}
	return models.TriggerEvent{
		TriggerID: codecs.FormatUint64ID(e.GetTriggerId()),
		EventType: e.GetEventType().String(),
		TsNs:      strconv.FormatUint(e.GetTsNs(), 10),
	}
}
