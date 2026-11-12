package services

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// brokenAlertAnswers are the broken answers of a mutation: no alert, or an
// alert with no ID.
var brokenAlertAnswers = map[string]func(*alertsv1.PriceAlert) *alertsv1.PriceAlert{
	"missing": func(*alertsv1.PriceAlert) *alertsv1.PriceAlert { return nil },
	"empty":   func(*alertsv1.PriceAlert) *alertsv1.PriceAlert { return &alertsv1.PriceAlert{} },
}

var allUpdateOutcomes = []alertsv1.UpdateOutcome{
	alertsv1.UpdateOutcome_UPDATE_OUTCOME_UNSPECIFIED,
	alertsv1.UpdateOutcome_APPLIED,
	alertsv1.UpdateOutcome_UNCHANGED,
	alertsv1.UpdateOutcome_REPLAYED,
}

func wantResponseContract(t *testing.T, err error, operation string) {
	t.Helper()
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) {
		t.Fatalf("err = %T %v, want *errors.ResponseContractError", err, err)
	}
	if contract.Operation != operation {
		t.Fatalf("operation = %q, want %q", contract.Operation, operation)
	}
	if !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("MutationOutcomeUnknown(%v) = false", err)
	}
}

// POLY-9918: a create, update or cancel answer without its alert breaks the
// response contract; it never reads as a zero-ID alert.
func TestAlertsMutationsRefuseAnAnswerWithoutTheAlert(t *testing.T) {
	for name, answer := range brokenAlertAnswers {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAlerts{answer: answer}
			service := newAlertsService(t, fake, alertsCatalog(t))
			ctx := context.Background()

			got, err := service.Create(ctx, CreateAlertParams{
				Symbol: ptr("BTC-USDT"), Direction: "above", Price: models.PriceFromDecimal("1000000"), Channels: []string{"in_app"},
			})
			wantResponseContract(t, err, "CreateAlert")
			if got.AlertID != "" {
				t.Fatalf("create returned %+v", got)
			}

			cancelled, err := service.Cancel(ctx, "123")
			wantResponseContract(t, err, "CancelAlert")
			if cancelled.AlertID != "" {
				t.Fatalf("cancel returned %+v", cancelled)
			}

			price := models.PriceFromDecimal("1000000")
			for _, outcome := range allUpdateOutcomes {
				fake.mu.Lock()
				fake.outcome = &outcome
				fake.mu.Unlock()
				result, err := service.Update(ctx, UpdateAlertParams{AlertID: "123", ExpectedRevision: 1, Price: &price})
				wantResponseContract(t, err, "UpdateAlert")
				if result.Outcome != "" || result.Alert.AlertID != "" {
					t.Fatalf("update (%v) returned %+v", outcome, result)
				}
			}
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.creates) != 1 || len(fake.cancels) != 1 || len(fake.updates) != len(allUpdateOutcomes) {
				t.Fatalf("creates=%d cancels=%d updates=%d", len(fake.creates), len(fake.cancels), len(fake.updates))
			}
		})
	}
}

// A populated update answer keeps its alert and outcome, whatever the outcome.
func TestAlertsUpdateReadsAPopulatedAlertForEveryOutcome(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, alertsCatalog(t))
	price := models.PriceFromDecimal("1000000")
	want := map[alertsv1.UpdateOutcome]string{
		alertsv1.UpdateOutcome_UPDATE_OUTCOME_UNSPECIFIED: "unspecified",
		alertsv1.UpdateOutcome_APPLIED:                    "applied",
		alertsv1.UpdateOutcome_UNCHANGED:                  "unchanged",
		alertsv1.UpdateOutcome_REPLAYED:                   "replayed",
	}
	for _, outcome := range allUpdateOutcomes {
		fake.mu.Lock()
		fake.outcome = &outcome
		fake.mu.Unlock()
		result, err := service.Update(context.Background(), UpdateAlertParams{AlertID: "123", ExpectedRevision: 1, Price: &price})
		if err != nil {
			t.Fatalf("%v: %v", outcome, err)
		}
		if result.Alert.AlertID != codecs.FormatUint64ID(123) || result.Alert.SymbolID != 1 || result.Alert.Revision != 2 || result.Outcome != want[outcome] {
			t.Fatalf("%v: result = %+v", outcome, result)
		}
	}
}

func btcEthCatalog(t *testing.T) *catalogs.Manager {
	t.Helper()
	manager := catalogs.NewManager()
	hydrateBTCAndETH(t, manager)
	return manager
}

func hydrateBTCAndETH(t *testing.T, manager *catalogs.Manager) {
	t.Helper()
	pair := func(symbol string, id float64) map[string]any {
		return map[string]any{
			"symbol": symbol, "symbol_id": id,
			"base_quantity_scale": float64(3), "quote_quantity_scale": float64(2),
			"tick_size": "0.01", "step_size": "0.001",
		}
	}
	if err := manager.HydrateSpotConfig(map[string]any{"pairs": []any{pair("BTC-USDT", 1), pair("ETH-USDT", 2)}}); err != nil {
		t.Fatal(err)
	}
}

func ethBoundPrice() models.PriceInput {
	return models.PriceFromTicks(models.MustPriceTicks(3_000_000_000_000).WithSymbol("ETH-USDT"))
}

func createSent(t *testing.T, fake *fakeAlerts) []*alertsv1.CreateAlertRequest {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]*alertsv1.CreateAlertRequest(nil), fake.creates...)
}

func wantValidation(t *testing.T, err error, contains string) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) {
		t.Fatalf("err = %T %v, want *errors.ValidationError", err, err)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("err = %v, want it to name %q", err, contains)
	}
}

// POLY-9919: a price bound to ETH-USDT is never sent for symbol_id 1 when the
// catalogs can't name that ID: a caller's Symbol beside it proves nothing.
func TestAlertsCreateRefusesABoundPriceTheCatalogsCannotCheck(t *testing.T) {
	selectors := map[string]CreateAlertParams{
		"id only":                  {SymbolID: ptr(uint32(1))},
		"matching symbol, BTC id":  {Symbol: ptr("ETH-USDT"), SymbolID: ptr(uint32(1))},
		"other symbol and same id": {Symbol: ptr("BTC-USDT"), SymbolID: ptr(uint32(1))},
	}
	managers := map[string]func() *catalogs.Manager{
		"empty catalogs": catalogs.NewManager,
		"no catalogs":    func() *catalogs.Manager { return nil },
	}
	for managerName, manager := range managers {
		for name, params := range selectors {
			t.Run(managerName+"/"+name, func(t *testing.T) {
				fake := &fakeAlerts{}
				service := newAlertsService(t, fake, manager())
				params.Direction, params.Price, params.Channels = "below", ethBoundPrice(), []string{"in_app"}
				_, err := service.Create(context.Background(), params)
				wantValidation(t, err, "ETH-USDT")
				if sent := createSent(t, fake); len(sent) != 0 {
					t.Fatalf("sent %v", sent)
				}
			})
		}
	}
}

// A load still in progress is waited for, and then checks the price.
func TestAlertsCreateWaitsForAPendingCatalogLoadToCheckABoundPrice(t *testing.T) {
	manager := catalogs.NewManager()
	finish := manager.BeginHydration()
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, manager)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := service.Create(ctx, CreateAlertParams{SymbolID: ptr(uint32(1)), Direction: "below", Price: ethBoundPrice(), Channels: []string{"in_app"}})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("create returned before the catalog load ended: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if sent := createSent(t, fake); len(sent) != 0 {
		t.Fatalf("sent during the load: %v", sent)
	}
	hydrateBTCAndETH(t, manager)
	finish(nil)
	wantValidation(t, <-done, "ETH-USDT")
	if sent := createSent(t, fake); len(sent) != 0 {
		t.Fatalf("sent %v", sent)
	}
	if _, err := service.Create(ctx, CreateAlertParams{SymbolID: ptr(uint32(2)), Direction: "below", Price: ethBoundPrice(), Channels: []string{"in_app"}}); err != nil {
		t.Fatal(err)
	}
	if sent := createSent(t, fake); len(sent) != 1 || sent[0].GetSymbolId() != 2 || sent[0].GetPriceTicks() != 3_000_000_000_000 {
		t.Fatalf("sent %v", sent)
	}
}

// A failed load leaves nothing to check a bound price against.
func TestAlertsCreateRefusesABoundPriceAfterAFailedCatalogLoad(t *testing.T) {
	manager := catalogs.NewManager()
	manager.BeginHydration()(stderrors.New("spot config unavailable"))
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, manager)
	_, err := service.Create(context.Background(), CreateAlertParams{SymbolID: ptr(uint32(1)), Direction: "below", Price: ethBoundPrice(), Channels: []string{"in_app"}})
	wantValidation(t, err, "symbol_id 1")
	if sent := createSent(t, fake); len(sent) != 0 {
		t.Fatalf("sent %v", sent)
	}
	// An unbound price needs no catalogs.
	if _, err := service.Create(context.Background(), CreateAlertParams{SymbolID: ptr(uint32(1)), Direction: "below", Price: models.PriceFromTicksInt(3_000_000_000_000), Channels: []string{"in_app"}}); err != nil {
		t.Fatal(err)
	}
}

func TestAlertsCreateChecksABoundPriceAgainstTheCataloguedMarket(t *testing.T) {
	refused := map[string]CreateAlertParams{
		"id only":     {SymbolID: ptr(uint32(1))},
		"conflicting": {Symbol: ptr("ETH-USDT"), SymbolID: ptr(uint32(1))},
	}
	for name, params := range refused {
		t.Run("refuses/"+name, func(t *testing.T) {
			fake := &fakeAlerts{}
			service := newAlertsService(t, fake, btcEthCatalog(t))
			params.Direction, params.Price, params.Channels = "below", ethBoundPrice(), []string{"in_app"}
			_, err := service.Create(context.Background(), params)
			var validation *sdkerrors.ValidationError
			if !stderrors.As(err, &validation) {
				t.Fatalf("err = %T %v, want *errors.ValidationError", err, err)
			}
			if sent := createSent(t, fake); len(sent) != 0 {
				t.Fatalf("sent %v", sent)
			}
		})
	}
	sent := map[string]CreateAlertParams{
		"id only":     {SymbolID: ptr(uint32(2))},
		"symbol only": {Symbol: ptr("ETH-USDT")},
		"both":        {Symbol: ptr("ETH-USDT"), SymbolID: ptr(uint32(2))},
	}
	for name, params := range sent {
		t.Run("sends/"+name, func(t *testing.T) {
			fake := &fakeAlerts{}
			service := newAlertsService(t, fake, btcEthCatalog(t))
			params.Direction, params.Price, params.Channels = "below", ethBoundPrice(), []string{"in_app"}
			if _, err := service.Create(context.Background(), params); err != nil {
				t.Fatal(err)
			}
			if got := createSent(t, fake); len(got) != 1 || got[0].GetSymbolId() != 2 {
				t.Fatalf("sent %v", got)
			}
		})
	}
}

// Unbound prices keep working without catalogs, by ID or by symbol and ID.
func TestAlertsCreateSendsAnUnboundPriceWithEmptyCatalogs(t *testing.T) {
	for name, params := range map[string]CreateAlertParams{
		"id only":       {SymbolID: ptr(uint32(1))},
		"symbol and id": {Symbol: ptr("ETH-USDT"), SymbolID: ptr(uint32(1))},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAlerts{}
			service := newAlertsService(t, fake, catalogs.NewManager())
			params.Direction, params.Price, params.Channels = "below", models.PriceFromTicksInt(3_000_000_000_000), []string{"in_app"}
			if _, err := service.Create(context.Background(), params); err != nil {
				t.Fatal(err)
			}
			if got := createSent(t, fake); len(got) != 1 || got[0].GetSymbolId() != 1 || got[0].GetPriceTicks() != 3_000_000_000_000 {
				t.Fatalf("sent %v", got)
			}
		})
	}
}

// The update names the alert, not its market: a bound price can't be checked
// without reading the alert, so it is refused before sending, even one bound
// to the alert's own market. Unbound prices are sent.
func TestAlertsUpdateRefusesAMarketBoundPrice(t *testing.T) {
	fake := &fakeAlerts{}
	service := newAlertsService(t, fake, btcEthCatalog(t))
	for name, price := range map[string]models.PriceInput{
		"other market": ethBoundPrice(),
		"alert market": models.PriceFromTicks(models.MustPriceTicks(90_000_000_000_000).WithSymbol("BTC-USDT")),
	} {
		_, err := service.Update(context.Background(), UpdateAlertParams{AlertID: "123", ExpectedRevision: 1, Price: &price})
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) || !strings.Contains(err.Error(), "unbound price") {
			t.Fatalf("%s: err = %T %v, want *errors.ValidationError naming an unbound price", name, err, err)
		}
	}
	fake.mu.Lock()
	sent := len(fake.updates)
	fake.mu.Unlock()
	if sent != 0 {
		t.Fatalf("sent %d updates with a bound price", sent)
	}
	for name, price := range map[string]models.PriceInput{
		"ticks":   models.PriceFromTicksInt(91_000_000_000_000),
		"decimal": models.PriceFromDecimal("91000"),
	} {
		if _, err := service.Update(context.Background(), UpdateAlertParams{AlertID: "123", ExpectedRevision: 1, Price: &price}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updates) != 2 || fake.updates[0].GetUpdate().GetPriceTicks() != 91_000_000_000_000 || fake.updates[1].GetUpdate().GetPriceTicks() != 91_000_000_000_000 {
		t.Fatalf("updates = %v", fake.updates)
	}
}
