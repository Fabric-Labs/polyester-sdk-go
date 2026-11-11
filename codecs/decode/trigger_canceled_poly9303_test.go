package decode

import (
	stderrors "errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// POLY-9303: a canceled trigger reads "canceled", the spelling of its
// canceled event and of every other SDK. "cancelled" is not a filter value.
func TestPoly9303CanceledTriggerStatusUsesContractSpelling(t *testing.T) {
	trigger := TriggerFromProto(&triggersv1.Trigger{TriggerId: 7, Status: triggersv1.TriggerStatus_STATUS_CANCELED})
	if trigger.Status != "canceled" {
		t.Fatalf("Status = %q, want canceled", trigger.Status)
	}
	if got := TriggerStatusLabel(triggersv1.TriggerStatus_STATUS_CANCELED); got != "canceled" {
		t.Fatalf("TriggerStatusLabel = %q, want canceled", got)
	}
	if got, err := TriggerStatusFromLabel("canceled"); err != nil || got != triggersv1.TriggerStatus_STATUS_CANCELED {
		t.Fatalf("TriggerStatusFromLabel(canceled) = %v, %v", got, err)
	}
	if got, err := TriggerEventTypeFromLabel("canceled"); err != nil || got != triggersv1.TriggerEventType_EVENT_CANCELED {
		t.Fatalf("TriggerEventTypeFromLabel(canceled) = %v, %v", got, err)
	}
}

func TestPoly9303CancelledSpellingIsRefusedAsValidationError(t *testing.T) {
	var validation *sdkerrors.ValidationError
	if _, err := TriggerStatusFromLabel("cancelled"); !stderrors.As(err, &validation) {
		t.Fatalf("TriggerStatusFromLabel(cancelled) err = %T %v, want ValidationError", err, err)
	}
	if _, err := TriggerEventTypeFromLabel("cancelled"); !stderrors.As(err, &validation) {
		t.Fatalf("TriggerEventTypeFromLabel(cancelled) err = %T %v, want ValidationError", err, err)
	}
}
