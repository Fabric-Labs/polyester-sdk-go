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
