package catalogs

import (
	"math"
	"testing"
)

// A catalog integer is never saturated or rounded: a float64 is taken only
// while every integer up to it is exact (2^53-1), and anything out of range
// is refused.
func TestParseUintExactBoundaries(t *testing.T) {
	const maxSafe = 1<<53 - 1
	for _, tc := range []struct {
		name string
		in   any
		want uint64
		ok   bool
	}{
		{"float 2^53-1", float64(maxSafe), maxSafe, true},
		{"float 2^53", float64(1 << 53), 0, false},
		{"float 2^53+1 (rounds to 2^53)", float64(9007199254740993), 0, false},
		{"float 2^53+2", float64(9007199254740994), 0, false},
		{"float 2^63", float64(1 << 63), 0, false},
		{"float 1e19", 1e19, 0, false},
		{"float 2^64", float64(18446744073709551616), 0, false},
		{"float +Inf", math.Inf(1), 0, false},
		{"float NaN", math.NaN(), 0, false},
		{"float 1.5", 1.5, 0, false},
		{"float -1", -1.0, 0, false},
		{"uint64 2^63", uint64(1 << 63), 1 << 63, true},
		{"uint64 max", uint64(math.MaxUint64), math.MaxUint64, true},
		{"string 2^53+1", "9007199254740993", 9007199254740993, true},
		{"string 2^63", "9223372036854775808", 1 << 63, true},
		{"string 2^64-1", "18446744073709551615", math.MaxUint64, true},
		{"string 2^64", "18446744073709551616", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseUintExact(tc.in, "field")
			if tc.ok {
				if err != nil || got != tc.want {
					t.Fatalf("got (%d, %v), want %d", got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("got %d, want an error", got)
			}
		})
	}
}

// A catalog scale that is not an exact integer is not a scale: the getter
// reports it missing instead of reading it as 0 or truncating it.
func TestInexactOptionalScaleReadsAsMissing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{"whole", float64(2), 2, true},
		{"zero", float64(0), 0, true},
		{"fraction", 1.9, 0, false},
		{"above 2^53", float64(1 << 60), 0, false},
		{"negative", -1.0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager()
			if err := m.HydrateSpotConfig(map[string]any{
				"pairs": []any{map[string]any{
					"symbol": "BTC-USDT", "symbol_id": float64(1),
					"base_quantity_scale": float64(8), "quote_quantity_scale": float64(6),
					"reference_price_scale": tc.value,
				}},
			}); err != nil {
				t.Fatal(err)
			}
			got, ok := m.ReferencePriceScaleForSymbolID(1)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
