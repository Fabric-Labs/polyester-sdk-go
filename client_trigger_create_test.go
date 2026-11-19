package polyester

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Triggers.Create refuses a stop-loss or take-profit with no trigger price
// before sending, with the code the API answers for trigger_price_ticks 0.
// With a trigger price the create is sent with it.
func TestTriggersCreateRefusesAStopWithoutATriggerPrice(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	if err := client.Catalogs.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)},
	}}); err != nil {
		t.Fatal(err)
	}
	create := func(kind string, triggerPrice *models.PriceInput) error {
		_, err := client.Triggers.Create(ctx, nil, "BTC-USDT", kind, triggerPrice, "sell", models.QtyFromDecimal("0.001"), "market", nil, "last", "", nil, nil, false, codecs.CreateTriggerOptions{})
		return err
	}
	for _, kind := range []string{"stop_loss", "take_profit"} {
		err := create(kind, nil)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) || !isValidationNaming(err, "trigger_price") {
			t.Fatalf("%s: err = %T %v, want a ValidationError naming trigger_price", kind, err, err)
		}
		if validation.Code != orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID.String() {
			t.Errorf("%s: code %q, want ERROR_CODE_TRIGGER_PRICE_INVALID", kind, validation.Code)
		}
		if sent := capture.take(); len(sent) != 0 {
			t.Errorf("%s: sent %+v", kind, sent)
		}

		price := models.PriceFromDecimal("50000")
		_ = create(kind, &price)
		sent := capture.take()
		if len(sent) != 1 {
			t.Fatalf("%s with a trigger price: sent %d requests", kind, len(sent))
		}
		intent := sent[0].msg.(*triggersv1.CreateTriggerRequest).GetTrigger()
		cond := intent.GetStopLoss()
		if kind == "take_profit" {
			cond = intent.GetTakeProfit()
		}
		if got := cond.GetTriggerPriceTicks(); got != 50_000_000_000_000 {
			t.Errorf("%s with a trigger price: sent trigger_price_ticks %d", kind, got)
		}
	}
}
