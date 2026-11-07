package codecs

import (
	"math"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestResolveOrderQuantityRefusesBeyondInt64AtScale18(t *testing.T) {
	resolvers := map[string]func(models.QtyInput) (int64, error){
		"base": func(in models.QtyInput) (int64, error) {
			return ResolveQtyScaled(in, 18, "qty", "", nil)
		},
		"quote": func(in models.QtyInput) (int64, error) {
			return ResolveQuoteQtyScaled(in, 18, "max_quote_debit", "", nil)
		},
	}
	for name, resolve := range resolvers {
		got, err := resolve(models.QtyFromDecimal("9.223372036854775807"))
		if err != nil || got != math.MaxInt64 {
			t.Fatalf("%s at MaxInt64: got %d err=%v", name, got, err)
		}
		for _, qty := range []string{"10", "9.223372036854775808", "18.446744073709551615"} {
			got, err := resolve(models.QtyFromDecimal(qty))
			if err == nil || !strings.Contains(err.Error(), "exceeds int64 range") {
				t.Fatalf("%s %s: got %d err=%v", name, qty, got, err)
			}
		}
	}
	// ParseQtyScaled keeps its uint64 result at scale 18.
	if got, err := ParseQtyScaled("10", 18, "qty"); err != nil || got != 10_000_000_000_000_000_000 {
		t.Fatalf("ParseQtyScaled: got %d err=%v", got, err)
	}
}

func TestResolveQtyScaledAcceptsWholeUnitsAtScaleZero(t *testing.T) {
	for _, qty := range []string{"1000000", "1000000.0"} {
		got, err := ResolveQtyScaled(models.QtyFromDecimal(qty), 0, "qty", "BONK-USDT", nil)
		if err != nil || got != 1000000 {
			t.Fatalf("%s: got %d err=%v", qty, got, err)
		}
	}
	if _, err := ResolveQtyScaled(models.QtyFromDecimal("0.5"), 0, "qty", "BONK-USDT", nil); err == nil ||
		err.Error() != "qty supports at most 0 decimal places: 0.5" {
		t.Fatalf("fractional at scale 0: %v", err)
	}
	if _, err := ResolveQtyScaled(models.QtyFromDecimal("1"), -1, "qty", "BONK-USDT", nil); err == nil ||
		!strings.Contains(err.Error(), "requires known quantity scale") {
		t.Fatalf("unknown scale: %v", err)
	}
}

func TestFormattersMinInt64(t *testing.T) {
	cases := []struct {
		value int64
		scale int
		want  string
	}{
		{math.MinInt64, 0, "-9223372036854775808"},
		{math.MinInt64, 9, "-9223372036.854775808"},
		{math.MinInt64, 18, "-9.223372036854775808"},
		{math.MinInt64, 36, "-0.000000000000000009223372036854775808"},
		{math.MinInt64 + 1, 0, "-9223372036854775807"},
		{math.MinInt64 + 1, 9, "-9223372036.854775807"},
		{math.MinInt64 + 1, 18, "-9.223372036854775807"},
		{math.MinInt64 + 1, 36, "-0.000000000000000009223372036854775807"},
		{math.MaxInt64, 18, "9.223372036854775807"},
		{-1, 9, "-0.000000001"},
		{0, 9, "0"},
	}
	for _, c := range cases {
		got, err := FormatQtyScaled(c.value, c.scale)
		if err != nil || got != c.want {
			t.Errorf("FormatQtyScaled(%d, %d) = %q, %v; want %q", c.value, c.scale, got, err, c.want)
		}
		if c.scale == PriceTickScale {
			if got := FormatPriceTicks(c.value); got != c.want {
				t.Errorf("FormatPriceTicks(%d) = %q; want %q", c.value, got, c.want)
			}
		}
	}
}
