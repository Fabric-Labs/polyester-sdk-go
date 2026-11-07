package models

import (
	"strings"
	"testing"
)

// A buy of 1500 scaled base units at price 2 with a 0.000001 quote fee, sized
// by a quote amount of 1 at quote scale 6. Its notional is 3e12 at scale
// base+9, so the exact totals are representable at every base scale.
func highScaleQuoteBuy() Order {
	return Order{
		CumQty:                       MustQtyScaled(1500),
		CumNotional:                  "3000000000000",
		FeeAsset:                     "quote",
		FeeTotal:                     &OrderFeeTotal{AmountE18: "1000000000000", Qty: MustQtyScaled(1500)},
		SubmittedMaxQuoteDebitScaled: "1000000",
	}
}

func TestExecutionTotalsAtEveryCatalogBaseScale(t *testing.T) {
	order := highScaleQuoteBuy()
	for _, scale := range []int{0, 8, 27, 28, 30, 36} {
		// k is the number of decimals of the notional, 3e(3-scale).
		k := scale - 3
		var cumQuote, used, unspent string
		switch {
		case k <= 0:
			cumQuote = "3" + strings.Repeat("0", -k)
			used = cumQuote + ".000001"
			unspent = "" // more than the budget
		case k < 6:
			cumQuote = "0." + strings.Repeat("0", k-1) + "3"
			used = cumQuote + strings.Repeat("0", 5-k) + "1"
			unspent = ""
		default:
			cumQuote = "0." + strings.Repeat("0", k-1) + "3"
			used = "0.000001" + strings.Repeat("0", k-7) + "3"
			unspent = "0.999998" + strings.Repeat("9", k-7) + "7"
		}
		if got, err := order.CumQuote(scale); err != nil || got != cumQuote {
			t.Errorf("CumQuote(%d) = %q, %v; want %q", scale, got, err, cumQuote)
		}
		if got, ok := order.QuoteUsed(scale); !ok || got != used {
			t.Errorf("QuoteUsed(%d) = %q, %v; want %q", scale, got, ok, used)
		}
		if unspent == "" {
			continue
		}
		if got, ok := order.QuoteUnspent(scale, 6); !ok || got != unspent {
			t.Errorf("QuoteUnspent(%d, 6) = %q, %v; want %q", scale, got, ok, unspent)
		}
	}
}

func TestExecutionTotalsRefuseScalesOutsideTheCatalogRange(t *testing.T) {
	order := highScaleQuoteBuy()
	for _, scale := range []int{-1, MaxProtocolScale + 1} {
		if got, err := order.CumQuote(scale); err == nil {
			t.Errorf("CumQuote(%d) = %q, want error", scale, got)
		}
		if got, ok := order.QuoteUsed(scale); ok {
			t.Errorf("QuoteUsed(%d) = %q, want not ok", scale, got)
		}
		if got, ok := order.QuoteUnspent(scale, 6); ok {
			t.Errorf("QuoteUnspent(%d, 6) = %q, want not ok", scale, got)
		}
	}
	for _, quoteScale := range []int{-1, MaxProtocolScale + 1} {
		if got, ok := order.QuoteUnspent(30, quoteScale); ok {
			t.Errorf("QuoteUnspent(30, %d) = %q, want not ok", quoteScale, got)
		}
	}
}
