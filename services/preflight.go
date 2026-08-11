package services

import (
	"fmt"
	"math/big"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func pairConstraints(cats *catalogs.Manager, symbol *string, symbolID *uint32) (models.PairConstraints, bool) {
	if cats == nil {
		return models.PairConstraints{}, false
	}
	if symbol != nil && *symbol != "" {
		return cats.PairConstraintsForSymbol(*symbol)
	}
	if symbolID != nil {
		return cats.PairConstraintsForSymbolID(*symbolID)
	}
	return models.PairConstraints{}, false
}

func preflightPairValues(
	constraints models.PairConstraints,
	qtyScaled int64,
	priceTicks []int64,
	notionalPriceTicks *int64,
) error {
	if qtyScaled > 0 {
		if constraints.StepSizeScaled > 0 && qtyScaled%constraints.StepSizeScaled != 0 {
			return &errors.ValidationError{Msg: fmt.Sprintf(
				"qty violates %s step_size %s", constraints.Symbol, constraints.StepSize,
			)}
		}
		if constraints.MinQtyScaled > 0 && qtyScaled < constraints.MinQtyScaled {
			return &errors.ValidationError{Msg: fmt.Sprintf(
				"qty is below %s min_qty_base %s", constraints.Symbol, constraints.MinQtyBase,
			)}
		}
	}
	for _, price := range priceTicks {
		if price > 0 && constraints.TickSizeTicks > 0 && price%constraints.TickSizeTicks != 0 {
			return &errors.ValidationError{Msg: fmt.Sprintf(
				"price violates %s tick_size %s", constraints.Symbol, constraints.TickSize,
			)}
		}
	}
	if qtyScaled <= 0 || notionalPriceTicks == nil || *notionalPriceTicks <= 0 ||
		!constraints.MinNotionalComputable {
		return nil
	}
	minNotional, ok := new(big.Rat).SetString(constraints.MinNotionalQuote)
	if !ok {
		return &errors.ValidationError{Msg: "catalog min_notional_quote is malformed"}
	}
	numerator := new(big.Int).Mul(big.NewInt(qtyScaled), big.NewInt(*notionalPriceTicks))
	denominator := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(constraints.BaseQuantityScale+6)),
		nil,
	)
	notional := new(big.Rat).SetFrac(numerator, denominator)
	if notional.Cmp(minNotional) < 0 {
		return &errors.ValidationError{Msg: fmt.Sprintf(
			"order notional is below %s min_notional_quote %s",
			constraints.Symbol, constraints.MinNotionalQuote,
		)}
	}
	return nil
}

func preflightOrderIntent(constraints models.PairConstraints, intent *orderv1.OrderIntent) error {
	if intent == nil {
		return nil
	}
	var prices []int64
	var notionalPrice *int64
	addPrice := func(value int64, computesNotional bool) {
		if value <= 0 {
			return
		}
		prices = append(prices, value)
		if computesNotional {
			v := value
			notionalPrice = &v
		}
	}
	switch {
	case intent.GetLimitGtc() != nil:
		addPrice(intent.GetLimitGtc().GetPriceTicks(), true)
	case intent.GetLimitIoc() != nil:
		addPrice(intent.GetLimitIoc().GetPriceTicks(), true)
	case intent.GetLimitFok() != nil:
		addPrice(intent.GetLimitFok().GetPriceTicks(), true)
	case intent.GetMarketIoc() != nil:
		addPrice(intent.GetMarketIoc().GetClientRefPriceTicks(), true)
	}
	return preflightPairValues(constraints, intent.GetBaseQtyScaled(), prices, notionalPrice)
}

func preflightTriggerIntent(constraints models.PairConstraints, intent *triggersv1.TriggerIntent) error {
	if intent == nil {
		return nil
	}
	var prices []int64
	var notionalPrice *int64
	addPrice := func(value int64, computesNotional bool) {
		if value <= 0 {
			return
		}
		prices = append(prices, value)
		if computesNotional {
			v := value
			notionalPrice = &v
		}
	}
	conditional := intent.GetStopLoss()
	if conditional == nil {
		conditional = intent.GetTakeProfit()
	}
	if conditional != nil {
		addPrice(conditional.GetTriggerPriceTicks(), false)
		child := conditional.GetChild()
		switch {
		case child.GetLimitGtc() != nil:
			addPrice(child.GetLimitGtc().GetPriceTicks(), true)
		case child.GetLimitIoc() != nil:
			addPrice(child.GetLimitIoc().GetPriceTicks(), true)
		case child.GetLimitFok() != nil:
			addPrice(child.GetLimitFok().GetPriceTicks(), true)
		}
	}
	if twap := intent.GetTwap(); twap != nil && twap.GetLimitGtc() != nil {
		addPrice(twap.GetLimitGtc().GetPriceTicks(), true)
	}
	if ladder := intent.GetLadder(); ladder != nil {
		addPrice(ladder.GetPriceMinTicks(), true)
		addPrice(ladder.GetPriceMaxTicks(), false)
	}
	if trailing := intent.GetTrailingStop(); trailing != nil {
		addPrice(trailing.GetActivationPriceTicks(), false)
	}
	return preflightPairValues(constraints, intent.GetQtyScaled(), prices, notionalPrice)
}
