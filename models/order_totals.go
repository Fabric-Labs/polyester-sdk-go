package models

import (
	"math/big"
	"strings"
)

// feeAmountScale is the fixed decimal scale of fee amounts (amount_e18) and
// of AvgPriceE18.
const feeAmountScale = 18

// AvgPrice formats AvgPriceE18 as an exact decimal price in quote units per
// base unit, with up to 18 decimal places and trailing zeros trimmed, as
// REST avgPx: "0" with no fills.
func (o Order) AvgPrice() string {
	value, ok := parseUnsigned(o.AvgPriceE18)
	if !ok {
		return "0"
	}
	out, err := formatFixedBig(value, feeAmountScale)
	if err != nil {
		return "0"
	}
	return out
}

// CumQuote formats CumNotional as an exact decimal quote amount, as REST
// cumQuote. baseQuantityScale is the pair's base_quantity_scale
// (catalogs.Manager.BaseQuantityScaleForSymbol). "0" with no fills. It is
// exactly the quote the fills settled when baseQuantityScale is at most 9;
// above that it can exceed the settled quote by less than 1e-18 per fill.
func (o Order) CumQuote(baseQuantityScale int) (string, error) {
	value, ok := parseUnsigned(o.CumNotional)
	if !ok {
		value = new(big.Int)
	}
	return formatFixedBig(value, PriceTickScale+baseQuantityScale)
}

// QuoteUsed is the quote amount the order's fills used, as an exact decimal:
// CumNotional, plus FeeTotal when the fee is paid in quote, or minus it when
// it is a rebate. A fee in base is taken from the base received, so it uses
// CumNotional only. ok is false until FeeTotal covers every fill (FeeTotal
// set and its Qty equal to CumQty): read the order again later, with
// execution history. baseQuantityScale is the pair's base_quantity_scale.
// Like CumQuote it is exact when baseQuantityScale is at most 9; above that
// it can exceed the settled debit by less than 1e-18 per fill, so allow that
// difference when reconciling it against a balance change.
func (o Order) QuoteUsed(baseQuantityScale int) (string, bool) {
	used, scale, ok := o.quoteUsed(baseQuantityScale)
	if !ok {
		return "", false
	}
	out, err := formatFixedBig(used, scale)
	return out, err == nil
}

// QuoteUnspent is what a buy sized by a quote amount did not use, as an exact
// decimal: SubmittedMaxQuoteDebitScaled at the pair's quoteQuantityScale less
// QuoteUsed. ok is false for an order not sized by a quote amount, and while
// QuoteUsed is not known.
func (o Order) QuoteUnspent(baseQuantityScale, quoteQuantityScale int) (string, bool) {
	budget, ok := parseUnsigned(o.SubmittedMaxQuoteDebitScaled)
	if !ok || quoteQuantityScale < 0 {
		return "", false
	}
	used, scale, ok := o.quoteUsed(baseQuantityScale)
	if !ok {
		return "", false
	}
	if quoteQuantityScale > scale {
		used = rescale(used, scale, quoteQuantityScale)
		scale = quoteQuantityScale
	}
	unspent := new(big.Int).Sub(rescale(budget, quoteQuantityScale, scale), used)
	out, err := formatFixedBig(unspent, scale)
	return out, err == nil
}

// quoteUsed returns the quote used as an integer at the returned scale.
func (o Order) quoteUsed(baseQuantityScale int) (*big.Int, int, bool) {
	if baseQuantityScale < 0 || o.FeeTotal == nil || o.FeeTotal.Qty.Scaled() != o.CumQty.Scaled() {
		return nil, 0, false
	}
	notional, ok := parseUnsigned(o.CumNotional)
	if !ok {
		notional = new(big.Int)
	}
	fee, ok := parseUnsigned(o.FeeTotal.AmountE18)
	if !ok {
		fee = new(big.Int)
	}
	notionalScale := PriceTickScale + baseQuantityScale
	scale := max(notionalScale, feeAmountScale)
	used := rescale(notional, notionalScale, scale)
	switch strings.ToLower(o.FeeAsset) {
	case "quote":
		fee = rescale(fee, feeAmountScale, scale)
		if o.FeeTotal.IsRebate {
			used.Sub(used, fee)
		} else {
			used.Add(used, fee)
		}
	case "base":
	default:
		if fee.Sign() != 0 {
			return nil, 0, false
		}
	}
	return used, scale, true
}

func parseUnsigned(raw string) (*big.Int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() < 0 {
		return nil, false
	}
	return value, true
}

// rescale returns value, at scale from, at scale to (to >= from).
func rescale(value *big.Int, from, to int) *big.Int {
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(to-from)), nil)
	return new(big.Int).Mul(value, factor)
}
