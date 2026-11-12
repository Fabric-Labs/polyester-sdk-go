package services

import (
	"context"
	"fmt"
	"iter"
	"regexp"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1/alertsv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// maxAlertsPageSize is the largest ListAlerts page the server serves; it
// answers 50 when the limit is 0.
const maxAlertsPageSize = 200

var alertIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// AlertsService manages durable price alerts (alerts.v1.PriceAlertService):
// "tell me when BTC-USDT falls to 90,000". An alert watches one market's last
// traded or index price, fires once when the price crosses its level, and is
// delivered to the in-app inbox or the account's linked Telegram account.
// The exchange evaluates alerts; a price that arrives late, or a feed that
// stops updating, never fires one.
//
// Price alerts are available on devnet; they are not yet routed on testnet,
// where every call fails. Alerts belong to the main account: an API key
// scoped to the main account manages them, and a key scoped to a sub-account
// is refused with an *errors.AuthError whose Code is
// errors.AlertCodeSubaccountKeyRefused. Read other refusals with
// errors.AlertErrorCode.
type AlertsService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
}

func NewAlertsService(factory *transport.Factory, cats *catalogs.Manager) *AlertsService {
	return &AlertsService{transport: factory, catalogs: cats}
}

func (s *AlertsService) client() alertsv1connect.PriceAlertServiceClient {
	return alertsv1connect.NewPriceAlertServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// CreateAlertParams describes a new price alert.
type CreateAlertParams struct {
	// Symbol or SymbolID names the market; given both, they must name the
	// same one. A Symbol is resolved through the loaded catalogs.
	Symbol   *string
	SymbolID *uint32
	// Direction is "above" or "below". Both are inclusive: "above" fires at a
	// price >= Price, "below" at a price <= Price.
	Direction string
	// Price is the alert's level, greater than zero.
	Price models.PriceInput
	// PriceSource is "last" (last traded price) or "index" (index price).
	// Empty lets the server choose: the index price where the market has
	// one, the last traded price otherwise. The returned alert states it.
	PriceSource string
	// Channels are where the alert is delivered: one or both of "in_app" and
	// "telegram" (the account's linked Telegram account).
	Channels []string
	// IdempotencyKey, when set, makes a retry of this create safe: the same
	// key and request return the alert created first. 1 to 128 characters
	// of A-Z, a-z, 0-9, '.', '_', ':' and '-'.
	IdempotencyKey string
}

// ListAlertsParams filters Alerts.List and Alerts.All.
type ListAlertsParams struct {
	// Statuses keeps alerts in any of these statuses: "active", "triggered",
	// "cancelled". Empty keeps every status.
	Statuses []string
	// Symbol or SymbolID keeps one market's alerts; neither keeps every
	// market.
	Symbol   *string
	SymbolID *uint32
	// Limit is the page size, at most 200; 0 lets the server use 50.
	Limit int
	// PageToken continues from a NextPageToken; keep the same filters.
	PageToken string
}

// UpdateAlertParams changes an alert. Only the fields set are sent and
// changed: Direction when not empty, Price when not nil, Channels when not
// nil. Price must be unbound (models.PriceFromDecimal or
// models.PriceFromTicksInt): a price bound to a market, such as an alert's own
// Price, is refused with an *errors.ValidationError before sending, since the
// update names the alert, not its market. With none set the update changes no field: it sets a fired
// ("triggered") alert again as it is, and leaves an active one unchanged.
type UpdateAlertParams struct {
	AlertID   string
	Direction string
	Price     *models.PriceInput
	Channels  []string
	// ExpectedRevision is the alert's Revision as last read. A stale
	// revision is refused with errors.AlertCodeRevisionConflict: read the
	// alert again.
	ExpectedRevision uint64
	// IdempotencyKey, when set, applies the update at most once: a retry or
	// replay with the same key and request returns the alert as it is now
	// with Outcome "replayed", even if it changed or fired since. Use one
	// key per intended change, and the same key for every retry of it.
	IdempotencyKey string
}

// Create creates a price alert. The market's current price is recorded as the
// alert's ReferencePrice; an alert whose condition already holds at that
// price starts unarmed and fires only after the price moves to the other side
// and crosses back.
//
// A price bound to a market (models.PriceTicks.WithSymbol) is checked against
// the catalogs' name for the SymbolID sent, after a catalog load in progress
// ends. When the catalogs can't name that ID (none were loaded, the load
// failed, or they don't list it) a bound price is refused with an
// *errors.ValidationError before anything is sent, whatever Symbol says:
// nothing proves which market the ID is. An unbound price is sent for any
// SymbolID. An answer without the created alert returns an
// *errors.ResponseContractError: the alert may exist, so list the alerts or
// create again with the same IdempotencyKey.
func (s *AlertsService) Create(ctx context.Context, params CreateAlertParams) (models.PriceAlert, error) {
	bound := boundPriceSymbol(&params.Price)
	if bound != "" {
		// The check below reads the catalogs: wait for a load in progress. A
		// failed load leaves the market unnamed, and the price is refused.
		if err := awaitCatalogs(ctx, s.catalogs, false); err != nil {
			return models.PriceAlert{}, err
		}
	}
	symbolID, market, err := s.resolveMarket(ctx, params.Symbol, params.SymbolID, "alerts.create", true)
	if err != nil {
		return models.PriceAlert{}, err
	}
	if bound != "" && market == "" {
		return models.PriceAlert{}, &errors.ValidationError{Msg: fmt.Sprintf(
			"alerts.create cannot check a price bound to %s against symbol_id %d: the market is not in the catalogs; hydrate catalogs or pass an unbound price",
			bound, symbolID,
		)}
	}
	direction, err := alertDirection(params.Direction)
	if err != nil {
		return models.PriceAlert{}, err
	}
	priceTicks, err := alertPriceTicks(params.Price, market)
	if err != nil {
		return models.PriceAlert{}, err
	}
	source, err := alertPriceSource(params.PriceSource)
	if err != nil {
		return models.PriceAlert{}, err
	}
	channels, err := alertChannels(params.Channels)
	if err != nil {
		return models.PriceAlert{}, err
	}
	if err := validateAlertIdempotencyKey(params.IdempotencyKey); err != nil {
		return models.PriceAlert{}, err
	}
	req := &alertsv1.CreateAlertRequest{
		SymbolId:       symbolID,
		Direction:      direction,
		PriceSource:    source,
		PriceTicks:     priceTicks,
		Channels:       channels,
		IdempotencyKey: params.IdempotencyKey,
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateAlert, req, func(msg *alertsv1.CreateAlertResponse) (models.PriceAlert, error) {
		return s.decodeAlert(msg.GetAlert(), "CreateAlert")
	})
}

// List returns one page of the account's alerts, newest first.
func (s *AlertsService) List(ctx context.Context, params ListAlertsParams) (models.PriceAlertsPage, error) {
	req, err := s.listRequest(ctx, params)
	if err != nil {
		return models.PriceAlertsPage{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ListAlerts, req, func(msg *alertsv1.ListAlertsResponse) models.PriceAlertsPage {
		return decode.PriceAlertsPageFromProto(msg, s.catalogs)
	})
}

// All iterates over every alert matching params, fetching pages as needed.
// Iteration stops at the first error.
func (s *AlertsService) All(ctx context.Context, params ListAlertsParams) iter.Seq2[models.PriceAlert, error] {
	return paginate(params.PageToken, func(token string) ([]models.PriceAlert, string, error) {
		params.PageToken = token
		page, err := s.List(ctx, params)
		return page.Alerts, page.NextPageToken, err
	})
}

// Update changes an alert's direction, price or channels; see
// UpdateAlertParams. Updating a triggered alert sets it again. An answer
// without the alert returns an *errors.ResponseContractError whatever its
// outcome: the update may have applied, so read the alert back.
func (s *AlertsService) Update(ctx context.Context, params UpdateAlertParams) (models.UpdatePriceAlertResult, error) {
	alertID, err := parseAlertID(params.AlertID)
	if err != nil {
		return models.UpdatePriceAlertResult{}, err
	}
	if params.ExpectedRevision == 0 {
		return models.UpdatePriceAlertResult{}, &errors.ValidationError{Msg: "expected_revision is required: pass the alert's Revision as last read"}
	}
	update := &alertsv1.AlertUpdate{}
	paths := []string{}
	if params.Direction != "" {
		if update.Direction, err = alertDirection(params.Direction); err != nil {
			return models.UpdatePriceAlertResult{}, err
		}
		paths = append(paths, "direction")
	}
	if params.Price != nil {
		// The update names the alert, not its market, so nothing proves a
		// bound price is for that market.
		if bound := boundPriceSymbol(params.Price); bound != "" {
			return models.UpdatePriceAlertResult{}, &errors.ValidationError{Msg: "alerts.update cannot check a price bound to " + bound +
				" against the alert's market; pass an unbound price: models.PriceFromDecimal or models.PriceFromTicksInt"}
		}
		if update.PriceTicks, err = alertPriceTicks(*params.Price, ""); err != nil {
			return models.UpdatePriceAlertResult{}, err
		}
		paths = append(paths, "price_ticks")
	}
	if params.Channels != nil {
		if update.Channels, err = alertChannels(params.Channels); err != nil {
			return models.UpdatePriceAlertResult{}, err
		}
		paths = append(paths, "channels")
	}
	if err := validateAlertIdempotencyKey(params.IdempotencyKey); err != nil {
		return models.UpdatePriceAlertResult{}, err
	}
	req := &alertsv1.UpdateAlertRequest{
		AlertId:          alertID,
		Update:           update,
		UpdateMask:       &fieldmaskpb.FieldMask{Paths: paths},
		ExpectedRevision: params.ExpectedRevision,
		IdempotencyKey:   params.IdempotencyKey,
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().UpdateAlert, req, func(msg *alertsv1.UpdateAlertResponse) (models.UpdatePriceAlertResult, error) {
		if err := requireAlert(msg.GetAlert(), "UpdateAlert"); err != nil {
			return models.UpdatePriceAlertResult{}, err
		}
		return decode.UpdatePriceAlertFromProto(msg, s.catalogs), nil
	})
}

// Cancel cancels an alert. Cancelling a cancelled alert returns it unchanged,
// so a retry is safe. An answer without the alert returns an
// *errors.ResponseContractError: cancel again or read the alert back.
func (s *AlertsService) Cancel(ctx context.Context, alertID string) (models.PriceAlert, error) {
	id, err := parseAlertID(alertID)
	if err != nil {
		return models.PriceAlert{}, err
	}
	req := &alertsv1.CancelAlertRequest{AlertId: id}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CancelAlert, req, func(msg *alertsv1.CancelAlertResponse) (models.PriceAlert, error) {
		return s.decodeAlert(msg.GetAlert(), "CancelAlert")
	})
}

func (s *AlertsService) decodeAlert(msg *alertsv1.PriceAlert, operation string) (models.PriceAlert, error) {
	if err := requireAlert(msg, operation); err != nil {
		return models.PriceAlert{}, err
	}
	return decode.PriceAlertFromProto(msg, s.catalogs), nil
}

// requireAlert refuses the answer of a create, update or cancel that has no
// alert, or an alert with no ID: it breaks the response contract, and the
// mutation may have applied.
func requireAlert(msg *alertsv1.PriceAlert, operation string) error {
	if msg == nil {
		return &errors.ResponseContractError{Operation: operation, Msg: "response has no alert"}
	}
	if msg.GetAlertId() == 0 {
		return &errors.ResponseContractError{Operation: operation, Msg: "response alert has no alert_id"}
	}
	return nil
}

func (s *AlertsService) listRequest(ctx context.Context, params ListAlertsParams) (*alertsv1.ListAlertsRequest, error) {
	if params.Limit < 0 || params.Limit > maxAlertsPageSize {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("limit must be from 0 to %d", maxAlertsPageSize)}
	}
	statuses := make([]alertsv1.AlertStatus, 0, len(params.Statuses))
	seen := map[alertsv1.AlertStatus]bool{}
	for _, label := range params.Statuses {
		status, err := alertStatus(label)
		if err != nil {
			return nil, err
		}
		if seen[status] {
			return nil, &errors.ValidationError{Msg: "statuses must not repeat a status: " + label}
		}
		seen[status] = true
		statuses = append(statuses, status)
	}
	symbolID, _, err := s.resolveMarket(ctx, params.Symbol, params.SymbolID, "alerts.list", false)
	if err != nil {
		return nil, err
	}
	return &alertsv1.ListAlertsRequest{
		Statuses:  statuses,
		SymbolId:  symbolID,
		Limit:     uint32(params.Limit),
		PageToken: params.PageToken,
	}, nil
}

// resolveMarket resolves a market selector, waiting for the catalog load
// when it names a market by Symbol. It returns the catalogs' name for the
// resolved ID when they know it, never the caller's Symbol.
func (s *AlertsService) resolveMarket(ctx context.Context, symbol *string, symbolID *uint32, label string, required bool) (uint32, string, error) {
	if namesSymbol(symbol) {
		if err := awaitCatalogs(ctx, s.catalogs, true); err != nil {
			return 0, "", err
		}
	}
	var id uint32
	var err error
	if required {
		id, err = ResolveSymbolID(s.catalogs, symbol, symbolID, label)
	} else {
		id, err = ResolveOptionalSymbolID(s.catalogs, symbol, symbolID, label)
	}
	if err != nil {
		return 0, "", err
	}
	if required && id == 0 {
		return 0, "", &errors.ValidationError{Msg: label + " requires symbol or symbol_id"}
	}
	name := ""
	if s.catalogs != nil && id != 0 {
		if known := s.catalogs.SymbolForSymbolID(id); known != nil {
			name = *known
		}
	}
	return id, name, nil
}

func parseAlertID(value string) (uint64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, &errors.ValidationError{Msg: "alert_id is required"}
	}
	id, err := codecs.IDToInt(value, "alert_id")
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, &errors.ValidationError{Msg: "alert_id must be a non-zero public ID"}
	}
	return id, nil
}

func alertDirection(label string) (alertsv1.AlertDirection, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "above":
		return alertsv1.AlertDirection_ABOVE, nil
	case "below":
		return alertsv1.AlertDirection_BELOW, nil
	case "":
		return 0, &errors.ValidationError{Msg: "direction is required: above or below"}
	default:
		return 0, &errors.ValidationError{Msg: "direction must be above or below, not " + label}
	}
}

func alertPriceSource(label string) (alertsv1.AlertPriceSource, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return alertsv1.AlertPriceSource_PRICE_SOURCE_UNSPECIFIED, nil
	case "last":
		return alertsv1.AlertPriceSource_LAST_PRICE, nil
	case "index":
		return alertsv1.AlertPriceSource_INDEX_PRICE, nil
	default:
		return 0, &errors.ValidationError{Msg: "price_source must be last or index, or empty for the server's choice, not " + label}
	}
}

func alertStatus(label string) (alertsv1.AlertStatus, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "active":
		return alertsv1.AlertStatus_ACTIVE, nil
	case "triggered":
		return alertsv1.AlertStatus_TRIGGERED, nil
	case "cancelled":
		return alertsv1.AlertStatus_CANCELLED, nil
	default:
		return 0, &errors.ValidationError{Msg: "status must be active, triggered or cancelled, not " + label}
	}
}

func alertChannels(labels []string) ([]alertsv1.AlertChannel, error) {
	if len(labels) == 0 || len(labels) > 2 {
		return nil, &errors.ValidationError{Msg: "channels must name one or two of in_app and telegram"}
	}
	out := make([]alertsv1.AlertChannel, 0, len(labels))
	for _, label := range labels {
		var channel alertsv1.AlertChannel
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "in_app":
			channel = alertsv1.AlertChannel_IN_APP
		case "telegram":
			channel = alertsv1.AlertChannel_TELEGRAM
		default:
			return nil, &errors.ValidationError{Msg: "channel must be in_app or telegram, not " + label}
		}
		for _, existing := range out {
			if existing == channel {
				return nil, &errors.ValidationError{Msg: "channels must not repeat a channel: " + label}
			}
		}
		out = append(out, channel)
	}
	return out, nil
}

func alertPriceTicks(price models.PriceInput, symbol string) (int64, error) {
	ticks, err := codecs.ResolvePriceTicks(price, "price", symbol)
	if err != nil {
		return 0, err
	}
	if ticks <= 0 {
		return 0, &errors.ValidationError{Msg: "price must be greater than zero"}
	}
	return ticks, nil
}

func validateAlertIdempotencyKey(key string) error {
	if key != "" && !alertIdempotencyKeyPattern.MatchString(key) {
		return &errors.ValidationError{Msg: "idempotency_key must be 1 to 128 characters of A-Z, a-z, 0-9, '.', '_', ':' and '-'"}
	}
	return nil
}
