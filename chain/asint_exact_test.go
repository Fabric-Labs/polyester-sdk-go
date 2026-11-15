package chain

import (
	"encoding/json"
	"math"
	"testing"
)

// asInt reads a JSON-RPC integer exactly or refuses it: a float64 is taken
// only when it is a whole number no larger in magnitude than 2^53-1, where
// every integer is exact, and is never truncated or saturated.
func TestAsIntRefusesInexactFloats(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"float 21000", float64(21000), "21000"},
		{"float 2^53-1", float64(1<<53 - 1), "9007199254740991"},
		{"float -(2^53-1)", -float64(1<<53 - 1), "-9007199254740991"},
		{"json.Number 2^64", json.Number("18446744073709551616"), "18446744073709551616"},
		{"string 2^63", "9223372036854775808", "9223372036854775808"},
		{"hex 2^64", "0x10000000000000000", "18446744073709551616"},
	} {
		got, err := asInt(tc.in)
		if err != nil || got.String() != tc.want {
			t.Errorf("%s: got (%v, %v), want %s", tc.name, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"float 1.9", 1.9},
		{"float 2^53", float64(1 << 53)},
		{"float 2^63", float64(1 << 63)},
		{"float 1e19", 1e19},
		{"float 2^64", float64(18446744073709551616)},
		{"float -1e19", -1e19},
		{"float NaN", math.NaN()},
		{"float +Inf", math.Inf(1)},
		{"json.Number 1.5", json.Number("1.5")},
	} {
		if got, err := asInt(tc.in); err == nil {
			t.Errorf("%s: got %v, want an error", tc.name, got)
		}
	}
}
