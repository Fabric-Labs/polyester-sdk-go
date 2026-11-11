package decode

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func alertDirectionLabel(v alertsv1.AlertDirection) string {
	switch v {
	case alertsv1.AlertDirection_ABOVE:
		return "above"
	case alertsv1.AlertDirection_BELOW:
		return "below"
	default:
		return "unspecified"
	}
}

func alertPriceSourceLabel(v alertsv1.AlertPriceSource) string {
	switch v {
	case alertsv1.AlertPriceSource_LAST_PRICE:
		return "last"
	case alertsv1.AlertPriceSource_INDEX_PRICE:
		return "index"
	default:
		return "unspecified"
	}
}

func alertStatusLabel(v alertsv1.AlertStatus) string {
	switch v {
	case alertsv1.AlertStatus_ACTIVE, alertsv1.AlertStatus_TRIGGERED, alertsv1.AlertStatus_CANCELLED:
		return strings.ToLower(v.String())
	default:
		return "unspecified"
	}
}

func alertChannelLabel(v alertsv1.AlertChannel) string {
	switch v {
	case alertsv1.AlertChannel_IN_APP:
		return "in_app"
	case alertsv1.AlertChannel_TELEGRAM:
		return "telegram"
	default:
		return "unspecified"
	}
}

func alertOutcomeLabel(v alertsv1.UpdateOutcome) string {
	switch v {
	case alertsv1.UpdateOutcome_APPLIED, alertsv1.UpdateOutcome_UNCHANGED, alertsv1.UpdateOutcome_REPLAYED:
		return strings.ToLower(v.String())
	default:
		return "unspecified"
	}
}

func alertPrice(ticks int64, symbol string) models.PriceTicks {
	if ticks < 0 {
		ticks = 0
	}
	return models.MustPriceTicks(ticks).WithSymbol(symbol)
}

// PriceAlertFromProto decodes one price alert, naming its market from cats
// when they know it.
func PriceAlertFromProto(msg *alertsv1.PriceAlert, cats *catalogs.Manager) models.PriceAlert {
	symbol := ""
	if cats != nil {
		if name := cats.SymbolForSymbolID(msg.GetSymbolId()); name != nil {
			symbol = *name
		}
	}
	channels := make([]string, 0, len(msg.GetChannels()))
	for _, channel := range msg.GetChannels() {
		channels = append(channels, alertChannelLabel(channel))
	}
	out := models.PriceAlert{
		AlertID:        codecs.FormatUint64ID(msg.GetAlertId()),
		SymbolID:       msg.GetSymbolId(),
		Symbol:         symbol,
		Direction:      alertDirectionLabel(msg.GetDirection()),
		PriceSource:    alertPriceSourceLabel(msg.GetPriceSource()),
		Price:          alertPrice(msg.GetPriceTicks(), symbol),
		ReferencePrice: alertPrice(msg.GetReferencePriceTicks(), symbol),
		Armed:          msg.GetArmed(),
		Status:         alertStatusLabel(msg.GetStatus()),
		Channels:       channels,
		Revision:       msg.GetRevision(),
		CreatedAt:      timestampTime(msg.GetCreatedAt()),
		UpdatedAt:      timestampTime(msg.GetUpdatedAt()),
		TriggeredAt:    timestampTime(msg.GetTriggeredAt()),
	}
	if msg.GetTriggeredAt() != nil || msg.GetTriggeredPriceTicks() != 0 {
		triggered := alertPrice(msg.GetTriggeredPriceTicks(), symbol)
		out.TriggeredPrice = &triggered
	}
	return out
}

// PriceAlertsPageFromProto decodes one ListAlerts page.
func PriceAlertsPageFromProto(msg *alertsv1.ListAlertsResponse, cats *catalogs.Manager) models.PriceAlertsPage {
	alerts := make([]models.PriceAlert, 0, len(msg.GetAlerts()))
	for _, alert := range msg.GetAlerts() {
		alerts = append(alerts, PriceAlertFromProto(alert, cats))
	}
	return models.PriceAlertsPage{Alerts: alerts, NextPageToken: msg.GetNextPageToken()}
}

// UpdatePriceAlertFromProto decodes an UpdateAlert answer.
func UpdatePriceAlertFromProto(msg *alertsv1.UpdateAlertResponse, cats *catalogs.Manager) models.UpdatePriceAlertResult {
	return models.UpdatePriceAlertResult{Alert: PriceAlertFromProto(msg.GetAlert(), cats), Outcome: alertOutcomeLabel(msg.GetOutcome())}
}
