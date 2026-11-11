package services

import (
	"context"
	"testing"
)

// POLY-9303: "canceled" is the only spelling of the canceled trigger status
// and event type; "cancelled" is refused before sending.
func TestPoly9303TriggerFiltersRefuseCancelledBeforeSending(t *testing.T) {
	t.Run("List", func(t *testing.T) {
		factory, captures := scopeCaptureFactory(t)
		triggers := NewTriggersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
		_, err := triggers.List(context.Background(), nil, nil, nil, []string{"cancelled"}, 10, nil)
		requireRefusedBeforeTransport(t, err, captures)
	})
	t.Run("ListEvents", func(t *testing.T) {
		factory, captures := scopeCaptureFactory(t)
		triggers := NewTriggersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
		_, err := triggers.ListEvents(context.Background(), nil, "9", nil, 10, new("cancelled"), nil)
		requireRefusedBeforeTransport(t, err, captures)
	})
}
