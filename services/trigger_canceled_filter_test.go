package services

import (
	"context"
	"testing"
)

// The canceled trigger status and event type are spelled only "canceled";
// "cancelled" is refused before sending.
func TestTriggerFiltersRefuseCancelledBeforeSending(t *testing.T) {
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
