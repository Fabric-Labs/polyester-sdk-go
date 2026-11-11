package services

import (
	"context"
	stderrors "errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1/alertsv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var alertCreatedAt = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)

type fakeAlerts struct {
	alertsv1connect.UnimplementedPriceAlertServiceHandler

	mu      sync.Mutex
	creates []*alertsv1.CreateAlertRequest
	lists   []*alertsv1.ListAlertsRequest
	updates []*alertsv1.UpdateAlertRequest
	cancels []*alertsv1.CancelAlertRequest
	signed  []bool
	// err, when set, answers every call.
	err error
}

func (f *fakeAlerts) record(h http.Header) {
	f.signed = append(f.signed, h.Get("X-API-SIGNATURE") != "" && h.Get("X-API-KEY-ID") != "")
}

func sampleAlert(id uint64) *alertsv1.PriceAlert {
	return &alertsv1.PriceAlert{
		AlertId:             id,
		SymbolId:            1,
		Direction:           alertsv1.AlertDirection_BELOW,
		PriceSource:         alertsv1.AlertPriceSource_INDEX_PRICE,
		PriceTicks:          90_000_000_000_000,
		ReferencePriceTicks: 95_123_450_000_000,
		Armed:               true,
		Status:              alertsv1.AlertStatus_ACTIVE,
		Channels:            []alertsv1.AlertChannel{alertsv1.AlertChannel_IN_APP, alertsv1.AlertChannel_TELEGRAM},
		Revision:            1,
		CreatedAt:           timestamppb.New(alertCreatedAt),
		UpdatedAt:           timestamppb.New(alertCreatedAt),
	}
}

func (f *fakeAlerts) CreateAlert(_ context.Context, req *connect.Request[alertsv1.CreateAlertRequest]) (*connect.Response[alertsv1.CreateAlertResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(req.Header())
	f.creates = append(f.creates, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&alertsv1.CreateAlertResponse{Alert: sampleAlert(1234)}), nil
}

func (f *fakeAlerts) ListAlerts(_ context.Context, req *connect.Request[alertsv1.ListAlertsRequest]) (*connect.Response[alertsv1.ListAlertsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(req.Header())
	f.lists = append(f.lists, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&alertsv1.ListAlertsResponse{Alerts: []*alertsv1.PriceAlert{sampleAlert(3), sampleAlert(2)}, NextPageToken: "p2"}), nil
	}
	triggered := sampleAlert(1)
	triggered.Status = alertsv1.AlertStatus_TRIGGERED
	triggered.Armed = false
	triggered.TriggeredAt = timestamppb.New(alertCreatedAt.Add(time.Hour))
	triggered.TriggeredPriceTicks = 89_999_000_000_000
	return connect.NewResponse(&alertsv1.ListAlertsResponse{Alerts: []*alertsv1.PriceAlert{triggered}}), nil
}

func (f *fakeAlerts) UpdateAlert(_ context.Context, req *connect.Request[alertsv1.UpdateAlertRequest]) (*connect.Response[alertsv1.UpdateAlertResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(req.Header())
	f.updates = append(f.updates, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	alert := sampleAlert(req.Msg.GetAlertId())
	alert.Revision = req.Msg.GetExpectedRevision() + 1
	return connect.NewResponse(&alertsv1.UpdateAlertResponse{Alert: alert, Outcome: alertsv1.UpdateOutcome_REPLAYED}), nil
}

func (f *fakeAlerts) CancelAlert(_ context.Context, req *connect.Request[alertsv1.CancelAlertRequest]) (*connect.Response[alertsv1.CancelAlertResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(req.Header())
	f.cancels = append(f.cancels, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	alert := sampleAlert(req.Msg.GetAlertId())
	alert.Status = alertsv1.AlertStatus_CANCELLED
	return connect.NewResponse(&alertsv1.CancelAlertResponse{Alert: alert}), nil
}

func alertsCatalog(t *testing.T) *catalogs.Manager {
	t.Helper()
	manager := catalogs.NewManager()
	if err := manager.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(1),
		"base_quantity_scale": float64(3), "quote_quantity_scale": float64(2),
		"tick_size": "0.01", "step_size": "0.001",
	}}}); err != nil {
		t.Fatal(err)
	}
	return manager
}

func newAlertsService(t *testing.T, fake *fakeAlerts, cats *catalogs.Manager) *AlertsService {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := alertsv1connect.NewPriceAlertServiceHandler(fake)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_alerts", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewAlertsService(transport.NewFactory(cfg, creds, nil), cats)
}

func ptr[T any](v T) *T { return &v }

func TestAlertsCreateSendsTheAlertAndDecodesIt(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, alertsCatalog(t))

	got, err := service.Create(context.Background(), CreateAlertParams{
		Symbol:         ptr("BTC-USDT"),
		Direction:      "below",
		Price:          models.PriceFromDecimal("90000"),
		Channels:       []string{"telegram"},
		IdempotencyKey: "run-7:tool-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.creates) != 1 || !fake.signed[0] {
		t.Fatalf("creates=%v signed=%v", fake.creates, fake.signed)
	}
	req := fake.creates[0]
	if req.GetSymbolId() != 1 || req.GetDirection() != alertsv1.AlertDirection_BELOW || req.GetPriceTicks() != 90_000_000_000_000 ||
		req.GetPriceSource() != alertsv1.AlertPriceSource_PRICE_SOURCE_UNSPECIFIED ||
		!reflect.DeepEqual(req.GetChannels(), []alertsv1.AlertChannel{alertsv1.AlertChannel_TELEGRAM}) ||
		req.GetIdempotencyKey() != "run-7:tool-2" {
		t.Fatalf("request = %v", req)
	}
	if got.AlertID != codecs.FormatUint64ID(1234) || got.SymbolID != 1 || got.Symbol != "BTC-USDT" ||
		got.Direction != "below" || got.PriceSource != "index" || got.Price.Format() != "90000" ||
		got.ReferencePrice.Format() != "95123.45" || !got.Armed || got.Status != "active" ||
		!reflect.DeepEqual(got.Channels, []string{"in_app", "telegram"}) || got.Revision != 1 ||
		got.CreatedAt == nil || !got.CreatedAt.Equal(alertCreatedAt) || got.TriggeredAt != nil || got.TriggeredPrice != nil {
		t.Fatalf("alert = %+v", got)
	}
}

func TestAlertsCreateSendsAnExplicitPriceSource(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, nil)
	if _, err := service.Create(context.Background(), CreateAlertParams{
		SymbolID: ptr(uint32(1)), Direction: "above", PriceSource: "last",
		Price: models.PriceFromTicksInt(100_000_000_000_000), Channels: []string{"in_app"},
	}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if req := fake.creates[0]; req.GetDirection() != alertsv1.AlertDirection_ABOVE || req.GetPriceSource() != alertsv1.AlertPriceSource_LAST_PRICE ||
		req.GetPriceTicks() != 100_000_000_000_000 || req.GetIdempotencyKey() != "" {
		t.Fatalf("request = %v", req)
	}
}

func TestAlertsCreateRefusesInvalidInputBeforeSending(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, alertsCatalog(t))
	valid := func() CreateAlertParams {
		return CreateAlertParams{SymbolID: ptr(uint32(1)), Direction: "above", Price: models.PriceFromDecimal("1"), Channels: []string{"in_app"}}
	}
	cases := map[string]func(*CreateAlertParams){
		"no market":            func(p *CreateAlertParams) { p.SymbolID = nil },
		"unknown market":       func(p *CreateAlertParams) { p.SymbolID = nil; p.Symbol = ptr("NOPE-USDT") },
		"no direction":         func(p *CreateAlertParams) { p.Direction = "" },
		"bad direction":        func(p *CreateAlertParams) { p.Direction = "sideways" },
		"zero price":           func(p *CreateAlertParams) { p.Price = models.PriceFromDecimal("0") },
		"no price":             func(p *CreateAlertParams) { p.Price = models.PriceInput{} },
		"bad price source":     func(p *CreateAlertParams) { p.PriceSource = "mark" },
		"no channel":           func(p *CreateAlertParams) { p.Channels = nil },
		"duplicate channel":    func(p *CreateAlertParams) { p.Channels = []string{"in_app", "in_app"} },
		"unknown channel":      func(p *CreateAlertParams) { p.Channels = []string{"email"} },
		"three channels":       func(p *CreateAlertParams) { p.Channels = []string{"in_app", "telegram", "in_app"} },
		"bad idempotency key":  func(p *CreateAlertParams) { p.IdempotencyKey = "has space" },
		"long idempotency key": func(p *CreateAlertParams) { p.IdempotencyKey = string(make([]byte, 129)) },
	}
	for name, mutate := range cases {
		params := valid()
		mutate(&params)
		_, err := service.Create(context.Background(), params)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %T %v, want *errors.ValidationError", name, err, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.creates) != 0 {
		t.Fatalf("sent %d invalid creates", len(fake.creates))
	}
}

func TestAlertsListFiltersAndAllWalksEveryPage(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, alertsCatalog(t))

	page, err := service.List(context.Background(), ListAlertsParams{Statuses: []string{"active", "triggered"}, Symbol: ptr("BTC-USDT"), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Alerts) != 2 || page.NextPageToken != "p2" || page.Alerts[0].AlertID != codecs.FormatUint64ID(3) {
		t.Fatalf("page = %+v", page)
	}
	var ids []string
	var last models.PriceAlert
	for alert, err := range service.All(context.Background(), ListAlertsParams{Statuses: []string{"active", "triggered"}, SymbolID: ptr(uint32(1)), Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, alert.AlertID)
		last = alert
	}
	want := []string{codecs.FormatUint64ID(3), codecs.FormatUint64ID(2), codecs.FormatUint64ID(1)}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if last.Status != "triggered" || last.Armed || last.TriggeredPrice == nil || last.TriggeredPrice.Format() != "89999" ||
		last.TriggeredAt == nil || !last.TriggeredAt.Equal(alertCreatedAt.Add(time.Hour)) {
		t.Fatalf("triggered alert = %+v", last)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.lists) != 3 {
		t.Fatalf("lists = %d", len(fake.lists))
	}
	wantStatuses := []alertsv1.AlertStatus{alertsv1.AlertStatus_ACTIVE, alertsv1.AlertStatus_TRIGGERED}
	for i, req := range fake.lists {
		if !reflect.DeepEqual(req.GetStatuses(), wantStatuses) || req.GetSymbolId() != 1 || req.GetLimit() != 2 {
			t.Fatalf("list %d = %v", i, req)
		}
	}
	if fake.lists[2].GetPageToken() != "p2" {
		t.Fatalf("second page token = %q", fake.lists[2].GetPageToken())
	}
}

func TestAlertsListRefusesInvalidFiltersBeforeSending(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, nil)
	for name, params := range map[string]ListAlertsParams{
		"limit over 200":   {Limit: 201},
		"negative limit":   {Limit: -1},
		"unknown status":   {Statuses: []string{"armed"}},
		"duplicate status": {Statuses: []string{"active", "active"}},
	} {
		_, err := service.List(context.Background(), params)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.lists) != 0 {
		t.Fatalf("sent %d invalid lists", len(fake.lists))
	}
}

func TestAlertsUpdateSendsOnlyTheFieldsItSets(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, nil)
	alertID := codecs.FormatUint64ID(1234)
	price := models.PriceFromDecimal("91000")

	got, err := service.Update(context.Background(), UpdateAlertParams{
		AlertID: alertID, Price: &price, Channels: []string{"in_app"}, ExpectedRevision: 1, IdempotencyKey: "run-7:tool-3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != "replayed" || got.Alert.AlertID != alertID || got.Alert.Revision != 2 {
		t.Fatalf("result = %+v", got)
	}
	// No field set: an empty mask, which sets a fired alert again.
	if _, err := service.Update(context.Background(), UpdateAlertParams{AlertID: alertID, ExpectedRevision: 3, IdempotencyKey: "run-7:tool-4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), UpdateAlertParams{AlertID: alertID, Direction: "above", ExpectedRevision: 4}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updates) != 3 {
		t.Fatalf("updates = %d", len(fake.updates))
	}
	first := fake.updates[0]
	if first.GetAlertId() != 1234 || first.GetExpectedRevision() != 1 || first.GetIdempotencyKey() != "run-7:tool-3" ||
		!reflect.DeepEqual(first.GetUpdateMask().GetPaths(), []string{"price_ticks", "channels"}) ||
		first.GetUpdate().GetPriceTicks() != 91_000_000_000_000 ||
		!reflect.DeepEqual(first.GetUpdate().GetChannels(), []alertsv1.AlertChannel{alertsv1.AlertChannel_IN_APP}) ||
		first.GetUpdate().GetDirection() != alertsv1.AlertDirection_DIRECTION_UNSPECIFIED {
		t.Fatalf("first update = %v", first)
	}
	rearm := fake.updates[1]
	if rearm.GetUpdateMask() == nil || len(rearm.GetUpdateMask().GetPaths()) != 0 || rearm.GetUpdate() == nil {
		t.Fatalf("re-arm update = %v", rearm)
	}
	if third := fake.updates[2]; !reflect.DeepEqual(third.GetUpdateMask().GetPaths(), []string{"direction"}) ||
		third.GetUpdate().GetDirection() != alertsv1.AlertDirection_ABOVE {
		t.Fatalf("direction update = %v", third)
	}
}

func TestAlertsUpdateAndCancelRefuseInvalidInputBeforeSending(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, nil)
	zero := models.PriceFromDecimal("0")
	for name, params := range map[string]UpdateAlertParams{
		"no alert id":         {ExpectedRevision: 1},
		"bad alert id":        {AlertID: "!!", ExpectedRevision: 1},
		"no revision":         {AlertID: codecs.FormatUint64ID(1)},
		"zero price":          {AlertID: codecs.FormatUint64ID(1), ExpectedRevision: 1, Price: &zero},
		"bad direction":       {AlertID: codecs.FormatUint64ID(1), ExpectedRevision: 1, Direction: "up"},
		"empty channels":      {AlertID: codecs.FormatUint64ID(1), ExpectedRevision: 1, Channels: []string{}},
		"bad idempotency key": {AlertID: codecs.FormatUint64ID(1), ExpectedRevision: 1, IdempotencyKey: "a/b"},
	} {
		_, err := service.Update(context.Background(), params)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
	for _, id := range []string{"", "!!", "0"} {
		_, err := service.Cancel(context.Background(), id)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("cancel %q: err = %T %v", id, err, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updates) != 0 || len(fake.cancels) != 0 {
		t.Fatalf("sent updates=%d cancels=%d", len(fake.updates), len(fake.cancels))
	}
}

func TestAlertsCancelReadsTheCancelledAlert(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, nil)
	got, err := service.Cancel(context.Background(), codecs.FormatUint64ID(77))
	if err != nil {
		t.Fatal(err)
	}
	if got.AlertID != codecs.FormatUint64ID(77) || got.Status != "cancelled" {
		t.Fatalf("alert = %+v", got)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.cancels) != 1 || fake.cancels[0].GetAlertId() != 77 || !fake.signed[0] {
		t.Fatalf("cancels = %v", fake.cancels)
	}
}

func alertRefusal(t *testing.T, code connect.Code, detailCode alertsv1.ErrorCode) error {
	t.Helper()
	err := connect.NewError(code, stderrors.New("refused"))
	detail, detailErr := connect.NewErrorDetail(&alertsv1.ErrorDetail{Code: detailCode})
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	err.AddDetail(detail)
	return err
}

func TestAlertsRefusalsKeepTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		code      connect.Code
		detail    alertsv1.ErrorCode
		want      string
		retryable bool
		authErr   bool
	}{
		{connect.CodeNotFound, alertsv1.ErrorCode_ALERT_NOT_FOUND, sdkerrors.AlertCodeNotFound, false, false},
		{connect.CodeAborted, alertsv1.ErrorCode_REVISION_CONFLICT, sdkerrors.AlertCodeRevisionConflict, false, false},
		{connect.CodeAlreadyExists, alertsv1.ErrorCode_IDEMPOTENCY_CONFLICT, sdkerrors.AlertCodeIdempotencyConflict, false, false},
		{connect.CodeFailedPrecondition, alertsv1.ErrorCode_ALERT_LIMIT_REACHED, sdkerrors.AlertCodeLimitReached, false, false},
		{connect.CodeFailedPrecondition, alertsv1.ErrorCode_ALERT_UPDATE_LIMIT_REACHED, sdkerrors.AlertCodeUpdateLimitReached, false, false},
		{connect.CodeFailedPrecondition, alertsv1.ErrorCode_ALERT_CANCELLED, sdkerrors.AlertCodeCancelled, false, false},
		{connect.CodeInvalidArgument, alertsv1.ErrorCode_UNKNOWN_SYMBOL, sdkerrors.AlertCodeUnknownSymbol, false, false},
		{connect.CodeUnavailable, alertsv1.ErrorCode_PRICE_UNAVAILABLE, sdkerrors.AlertCodePriceUnavailable, true, false},
		// Cancel is a mutation: an unavailable service leaves its outcome unknown.
		{connect.CodeUnavailable, alertsv1.ErrorCode_SERVICE_UNAVAILABLE, sdkerrors.AlertCodeServiceUnavailable, false, false},
		{connect.CodePermissionDenied, alertsv1.ErrorCode_SUBACCOUNT_KEY_REFUSED, sdkerrors.AlertCodeSubaccountKeyRefused, false, true},
	} {
		fake := &fakeAlerts{err: alertRefusal(t, tc.code, tc.detail)}
		service := newAlertsService(t, fake, nil)
		_, err := service.Cancel(context.Background(), codecs.FormatUint64ID(5))
		if got := sdkerrors.AlertErrorCode(err); got != tc.want {
			t.Fatalf("%v: AlertErrorCode = %q, want %q (err %T %v)", tc.detail, got, tc.want, err, err)
		}
		if sdkerrors.IsRetryable(err) != tc.retryable {
			t.Fatalf("%v: retryable = %v", tc.detail, sdkerrors.IsRetryable(err))
		}
		var authErr *sdkerrors.AuthError
		if stderrors.As(err, &authErr) != tc.authErr {
			t.Fatalf("%v: auth error = %v (err %T)", tc.detail, stderrors.As(err, &authErr), err)
		}
	}
}
