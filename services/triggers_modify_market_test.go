package services

import (
	"context"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A price bound to another market is refused before sending when the
// trigger's market is given, as trigger creation and order writes do.
func TestTriggerModifyRefusesAPriceForAnotherMarket(t *testing.T) {
	sol := models.PriceFromTicks(models.MustPriceTicks(46_000_000_000_000).WithSymbol("SOL-USDT"))
	symbol := "BTC-USDT"
	symbolID := uint32(5)
	triggerID := codecs.FormatID(9)
	for name, opts := range map[string]codecs.ModifyTriggerOptions{
		"trigger_price":    {TriggerPrice: &sol},
		"activation_price": {ActivationPrice: &sol},
	} {
		for selector, args := range map[string]struct {
			symbol   *string
			symbolID *uint32
		}{"symbol": {&symbol, nil}, "symbol_id": {nil, &symbolID}} {
			t.Run(name+"/"+selector, func(t *testing.T) {
				handler := &triggerControlHandler{}
				service := newTriggerControlService(t, handler)
				_, err := service.Modify(context.Background(), nil, triggerID, nil, args.symbol, args.symbolID, opts)
				if err == nil || !strings.Contains(err.Error(), "price symbol mismatch: value is for SOL-USDT, destination is BTC-USDT") {
					t.Fatalf("err=%v", err)
				}
				handler.mu.Lock()
				defer handler.mu.Unlock()
				if len(handler.modify) != 0 {
					t.Fatalf("sent %d modify requests", len(handler.modify))
				}
			})
		}
	}
}

func TestTriggerModifySendsAPriceForItsMarketOrAnUnknownOne(t *testing.T) {
	handler := &triggerControlHandler{}
	service := newTriggerControlService(t, handler)
	triggerID := codecs.FormatID(9)
	symbol := "BTC-USDT"
	btc := models.PriceFromTicks(models.MustPriceTicks(47_000_000_000_000).WithSymbol("BTC-USDT"))
	sol := models.PriceFromTicks(models.MustPriceTicks(46_000_000_000_000).WithSymbol("SOL-USDT"))
	if _, err := service.Modify(context.Background(), nil, triggerID, nil, &symbol, nil, codecs.ModifyTriggerOptions{TriggerPrice: &btc}); err != nil {
		t.Fatal(err)
	}
	// Without a symbol or symbol ID the trigger's market is not known here.
	if _, err := service.Modify(context.Background(), nil, triggerID, nil, nil, nil, codecs.ModifyTriggerOptions{TriggerPrice: &sol}); err != nil {
		t.Fatal(err)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.modify) != 2 ||
		handler.modify[0].GetTriggerPriceTicks() != 47_000_000_000_000 || handler.modify[0].GetSymbolId() != 5 ||
		handler.modify[1].GetTriggerPriceTicks() != 46_000_000_000_000 || handler.modify[1].GetSymbolId() != 0 {
		t.Fatalf("modify=%v", handler.modify)
	}
}

// An explicit symbol ID the catalogs don't know can't be checked against a
// bound price, so the price is refused rather than sent unchecked. An
// unbound price is still sent.
func TestTriggerModifyRefusesABoundPriceItCannotCheck(t *testing.T) {
	triggerID := codecs.FormatID(9)
	unknown := uint32(6)
	for priceName, price := range map[string]models.PriceInput{
		"btc_bound": models.PriceFromTicks(models.MustPriceTicks(47_000_000_000_000).WithSymbol("BTC-USDT")),
		"sol_bound": models.PriceFromTicks(models.MustPriceTicks(46_000_000_000_000).WithSymbol("SOL-USDT")),
	} {
		for field, opts := range map[string]codecs.ModifyTriggerOptions{
			"trigger_price":    {TriggerPrice: &price},
			"activation_price": {ActivationPrice: &price},
		} {
			t.Run(priceName+"/"+field, func(t *testing.T) {
				handler := &triggerControlHandler{}
				service := newTriggerControlService(t, handler)
				_, err := service.Modify(context.Background(), nil, triggerID, nil, nil, &unknown, opts)
				if err == nil || !strings.Contains(err.Error(), "cannot check a price bound to") {
					t.Fatalf("err=%v", err)
				}
				handler.mu.Lock()
				defer handler.mu.Unlock()
				if len(handler.modify) != 0 {
					t.Fatalf("sent %d modify requests", len(handler.modify))
				}
			})
		}
	}
	t.Run("no_catalogs", func(t *testing.T) {
		handler := &triggerControlHandler{}
		service := newTriggerControlService(t, handler)
		service.catalogs = nil
		btc := models.PriceFromTicks(models.MustPriceTicks(47_000_000_000_000).WithSymbol("BTC-USDT"))
		_, err := service.Modify(context.Background(), nil, triggerID, nil, nil, &unknown, codecs.ModifyTriggerOptions{TriggerPrice: &btc})
		if err == nil || !strings.Contains(err.Error(), "cannot check a price bound to") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("unbound", func(t *testing.T) {
		handler := &triggerControlHandler{}
		service := newTriggerControlService(t, handler)
		unbound := models.PriceFromTicksInt(46_000_000_000_000)
		if _, err := service.Modify(context.Background(), nil, triggerID, nil, nil, &unknown, codecs.ModifyTriggerOptions{TriggerPrice: &unbound, ActivationPrice: &unbound}); err != nil {
			t.Fatal(err)
		}
		handler.mu.Lock()
		defer handler.mu.Unlock()
		if len(handler.modify) != 1 || handler.modify[0].GetSymbolId() != 6 ||
			handler.modify[0].GetTriggerPriceTicks() != 46_000_000_000_000 || handler.modify[0].GetActivationPriceTicks() != 46_000_000_000_000 {
			t.Fatalf("modify=%v", handler.modify)
		}
	})
	t.Run("matching_by_id", func(t *testing.T) {
		handler := &triggerControlHandler{}
		service := newTriggerControlService(t, handler)
		known := uint32(5)
		btc := models.PriceFromTicks(models.MustPriceTicks(47_000_000_000_000).WithSymbol("BTC-USDT"))
		if _, err := service.Modify(context.Background(), nil, triggerID, nil, nil, &known, codecs.ModifyTriggerOptions{ActivationPrice: &btc}); err != nil {
			t.Fatal(err)
		}
	})
}
