//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestTriggerPauseResumeCancel(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	triggerPrice, limitPrice, qty, err := testutil.USDTFundedBuyStopParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	clientTriggerID := testutil.UniqueClientOrderID("trg")

	tp := models.PriceFromDecimal(triggerPrice)
	lp := models.PriceFromDecimal(limitPrice)
	created, err := client.Triggers.Create(ctx, nil, symbol, "stop_loss", &tp, "buy", models.QtyFromDecimal(qty), "limit", &lp, "", "", nil, &clientTriggerID, false, codecs.CreateTriggerOptions{})
	if err != nil {
		if testutil.DevnetBackendUnavailable(err) {
			t.Skipf("devnet trigger placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("trigger sizing below devnet minimum notional: %v", err)
		}
		t.Fatal(err)
	}
	if created.TriggerID == "" {
		t.Fatal("expected trigger_id from create")
	}
	// CreateTriggerResponse no longer carries a lifecycle status (POLY-3701);
	// SDKs synthesize "accepted" for admission.
	if created.Status != "accepted" {
		t.Fatalf("create status=%q want accepted", created.Status)
	}

	cancelledOK := false
	defer func() {
		if !cancelledOK {
			_, _ = client.Triggers.Cancel(ctx, nil, created.TriggerID, nil)
		}
	}()

	trigger, err := testutil.WaitForTrigger(ctx, client, created.TriggerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trigger.TriggerID != created.TriggerID {
		t.Fatalf("trigger_id=%q want %q", trigger.TriggerID, created.TriggerID)
	}
	if trigger.ClientTriggerID != clientTriggerID {
		t.Fatalf("client_trigger_id=%q want %q", trigger.ClientTriggerID, clientTriggerID)
	}
	if trigger.Status != "armed" {
		t.Fatalf("trigger status=%q want armed", trigger.Status)
	}

	paused, err := client.Triggers.Pause(ctx, nil, created.TriggerID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if paused.TriggerID != created.TriggerID {
		t.Fatalf("pause trigger_id=%q", paused.TriggerID)
	}
	if paused.Status != "paused" {
		t.Fatalf("pause status=%q want paused", paused.Status)
	}

	resumed, err := client.Triggers.Resume(ctx, nil, created.TriggerID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.TriggerID != created.TriggerID {
		t.Fatalf("resume trigger_id=%q", resumed.TriggerID)
	}
	if resumed.Status != "armed" {
		t.Fatalf("resume status=%q want armed", resumed.Status)
	}

	cancelled, err := client.Triggers.Cancel(ctx, nil, created.TriggerID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.TriggerID != created.TriggerID {
		t.Fatalf("cancel trigger_id=%q", cancelled.TriggerID)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancel status=%q want cancelled", cancelled.Status)
	}
	cancelledOK = true

	events, err := testutil.WaitForTriggerEvents(ctx, client, created.TriggerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events.Events {
		if event.TriggerID == created.TriggerID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected trigger events for cancelled trigger")
	}
}
