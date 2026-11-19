package codecs_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A stop-loss or take-profit takes a trigger price: the contract's
// trigger_price_ticks is above 0. With none, the create is refused before
// sending, as an explicit 0 is, naming trigger_price with the code the API
// answers. A valid price encodes unchanged.
func TestCreateTriggerRequiresAStopTriggerPrice(t *testing.T) {
	create := func(kind string, triggerPrice *models.PriceInput, orderType string, limitPrice *models.PriceInput) (*triggersv1.CreateTriggerRequest, error) {
		return codecs.CreateTriggerToProto(1, "BTC-USDT", kind, triggerPrice, "sell", models.QtyFromDecimal("0.001"), orderType, limitPrice, "last", "", nil, nil, false, 8, codecs.CreateTriggerOptions{})
	}
	price := func(v string) *models.PriceInput {
		p := models.PriceFromDecimal(v)
		return &p
	}
	unset := models.PriceInput{}
	conditional := func(kind string, req *triggersv1.CreateTriggerRequest) *triggersv1.ConditionalTrigger {
		if kind == "stop_loss" {
			return req.GetTrigger().GetStopLoss()
		}
		return req.GetTrigger().GetTakeProfit()
	}
	for _, kind := range []string{"stop_loss", "take_profit"} {
		for name, triggerPrice := range map[string]*models.PriceInput{"nil": nil, "unset": &unset, "explicit 0": price("0")} {
			req, err := create(kind, triggerPrice, "market", nil)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(validation.Msg, "trigger_price") {
				t.Fatalf("%s %s trigger price: err = %T %v, want a ValidationError naming trigger_price", kind, name, err, err)
			}
			if validation.Code != orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID.String() {
				t.Errorf("%s %s trigger price: code %q, want ERROR_CODE_TRIGGER_PRICE_INVALID", kind, name, validation.Code)
			}
			if req != nil {
				t.Errorf("%s %s trigger price: built %v", kind, name, req)
			}
		}

		_, err := create(kind, price("50000"), "limit", nil)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || validation.Msg != "limit trigger requires limit_price" {
			t.Errorf("%s limit child without limit_price: err = %v", kind, err)
		}

		req, err := create(kind, price("50000"), "market", nil)
		if err != nil {
			t.Fatalf("%s market: %v", kind, err)
		}
		if got := conditional(kind, req).GetTriggerPriceTicks(); got != 50_000_000_000_000 {
			t.Errorf("%s market: trigger_price_ticks %d", kind, got)
		}
		req, err = create(kind, price("50000"), "limit", price("49900"))
		if err != nil {
			t.Fatalf("%s limit: %v", kind, err)
		}
		cond := conditional(kind, req)
		if cond.GetTriggerPriceTicks() != 50_000_000_000_000 || cond.GetChild().GetLimitGtc().GetPriceTicks() != 49_900_000_000_000 {
			t.Errorf("%s limit: encoded %v", kind, cond)
		}
	}
}

// An OCO leg with no trigger price is refused the same way, naming the leg.
func TestCreateOcoRequiresEachLegsTriggerPrice(t *testing.T) {
	leg := models.RiskLegInput{TriggerPrice: models.PriceFromDecimal("50000")}
	for _, missing := range []string{"take_profit", "stop_loss"} {
		in := models.CreateOcoInput{Symbol: "BTC-USDT", Side: "sell", Qty: models.QtyFromDecimal("0.001"), TakeProfit: leg, StopLoss: leg}
		if missing == "take_profit" {
			in.TakeProfit = models.RiskLegInput{}
		} else {
			in.StopLoss = models.RiskLegInput{}
		}
		req, err := codecs.CreateOcoToProto(1, in, nil, 8)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || !strings.Contains(validation.Msg, missing+".trigger_price") {
			t.Fatalf("%s: err = %T %v, want a ValidationError naming %s.trigger_price", missing, err, err, missing)
		}
		if validation.Code != orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID.String() || req != nil {
			t.Errorf("%s: code %q, request %v", missing, validation.Code, req)
		}
	}
}
