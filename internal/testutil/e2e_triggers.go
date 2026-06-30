package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// USDTFundedBuyStopParams returns trigger, limit, and qty for a buy stop unlikely to fire.
func USDTFundedBuyStopParams(ctx context.Context, client *polyester.Client, symbol string) (triggerPrice, limitPrice, qty string, err error) {
	spotRaw, err := HydrateSpotRaw(ctx, client)
	if err != nil {
		return "", "", "", err
	}
	pair := PairForSymbol(spotRaw, symbol)
	triggerPrice = FarAboveBuyStopPrice(symbol, pair)
	limitPrice = strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRIGGER_LIMIT_PRICE"))
	if limitPrice == "" {
		limitPrice = triggerPrice
	}
	if qty = strings.TrimSpace(os.Getenv("POLYESTER_TEST_QTY")); qty == "" {
		qty = strings.TrimSpace(os.Getenv("POLYESTER_SMOKE_QTY"))
	}
	if qty == "" {
		qty = MinBaseQtyForPair(pair, limitPrice)
	}
	return triggerPrice, limitPrice, qty, nil
}

// WaitForTrigger polls until a trigger is readable.
func WaitForTrigger(ctx context.Context, client *polyester.Client, triggerID string, timeout time.Duration) (models.Trigger, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		trigger, err := client.Triggers.Get(ctx, nil, triggerID, nil)
		if err == nil && trigger != nil {
			return *trigger, nil
		}
		if err != nil && !IsNotFound(err) {
			return models.Trigger{}, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return models.Trigger{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return models.Trigger{}, fmt.Errorf("trigger %s was not readable within %s: %w", triggerID, timeout, lastErr)
}

// WaitForTriggerEvents polls until events for a trigger are visible.
func WaitForTriggerEvents(ctx context.Context, client *polyester.Client, triggerID string, timeout time.Duration) (models.TriggerEventsList, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events, err := client.Triggers.ListEvents(ctx, nil, triggerID, nil, 10, nil)
		if err != nil {
			return models.TriggerEventsList{}, err
		}
		for _, event := range events.Events {
			if event.TriggerID == triggerID {
				return events, nil
			}
		}
		select {
		case <-ctx.Done():
			return models.TriggerEventsList{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return models.TriggerEventsList{}, fmt.Errorf("trigger events for %s were not visible within %s", triggerID, timeout)
}
