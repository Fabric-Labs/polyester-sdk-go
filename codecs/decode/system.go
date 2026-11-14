package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// StatusEventsPageFromProto decodes one ListStatusEvents page. Events is
// empty, never nil.
func StatusEventsPageFromProto(msg *systemv1.ListStatusEventsResponse) models.StatusEventsPage {
	events := make([]models.StatusEvent, 0, len(msg.GetEvents()))
	for _, event := range msg.GetEvents() {
		if event == nil {
			continue
		}
		events = append(events, StatusEventFromProto(event))
	}
	return models.StatusEventsPage{Events: events, NextPageToken: msg.GetNextPageToken()}
}

// StatusEventFromProto decodes one status event. Omitted optional times and
// an omitted details URL stay nil.
func StatusEventFromProto(msg *systemv1.StatusEvent) models.StatusEvent {
	kind, state, impact := msg.GetKind(), msg.GetState(), msg.GetImpact()
	services := make([]string, 0, len(msg.GetServices()))
	for _, service := range msg.GetServices() {
		if label := codecs.ProtoEnumName(service, service.Number()); label != "" {
			services = append(services, label)
		}
	}
	var updates []models.StatusEventUpdate
	for _, update := range msg.GetUpdates() {
		if update == nil {
			continue
		}
		updates = append(updates, models.StatusEventUpdate{
			ID:        update.GetId(),
			Message:   update.GetMessage(),
			CreatedAt: timestampTime(update.GetCreatedAt()),
		})
	}
	return models.StatusEvent{
		ID:          codecs.FormatUint64ID(msg.GetId()),
		Kind:        codecs.ProtoEnumName(kind, kind.Number()),
		State:       codecs.ProtoEnumName(state, state.Number()),
		Title:       msg.GetTitle(),
		Message:     msg.GetMessage(),
		StartTime:   timestampTime(msg.GetStartTime()),
		EndTime:     timestampTime(msg.GetEndTime()),
		Services:    services,
		Markets:     cloneStrings(msg.GetMarkets()),
		Assets:      cloneStrings(msg.GetAssets()),
		Impact:      codecs.ProtoEnumName(impact, impact.Number()),
		UpdatedAt:   timestampTime(msg.GetUpdatedAt()),
		CancelledAt: timestampTime(msg.GetCancelledAt()),
		Updates:     updates,
		DetailsURL:  cloneOptionalString(msg.DetailsUrl),
	}
}

// cloneStrings copies a list; nil when it is empty.
func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
