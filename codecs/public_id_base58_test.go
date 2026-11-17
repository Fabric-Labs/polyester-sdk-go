package codecs

import "testing"

// A string public ID is base58, exactly as the API's own parser reads it: a
// string of digits is read as base58 too, never as a decimal number, so one
// string names one ID. A character outside the base58 alphabet, "0" or any
// whitespace included, is refused rather than trimmed or read as decimal.
func TestStringIDsAreBase58Only(t *testing.T) {
	for value, want := range map[string]uint64{
		"23":          60,
		"123":         60, // a leading "1" is a base58 zero digit
		"5":           4,
		"2nsoduvuPJr": 0,
	} {
		got, err := IDToInt(value, "order_id")
		if err != nil {
			t.Fatalf("IDToInt(%q): %v", value, err)
		}
		if want != 0 && got != want {
			t.Fatalf("IDToInt(%q) = %d, want %d (base58)", value, got, want)
		}
		if FormatID(got) != value && value[0] != '1' {
			t.Fatalf("IDToInt(%q) = %d does not round-trip through FormatID", value, got)
		}
	}
	for _, value := range []string{"100", "00023", "0", "10", "18446744073709551615", "9000000001", " 23", "23 ", "\t23", "2 3", "23\n"} {
		if id, err := IDToInt(value, "order_id"); err == nil {
			t.Errorf("IDToInt(%q) = %d, want a ValidationError: a decimal string is not a public ID", value, id)
		} else {
			refusedNaming(t, "order_id", err)
		}
		if id, err := ParseAccountID(value, "sub_account_id"); err == nil {
			t.Errorf("ParseAccountID(%q) = %d, want a ValidationError", value, id)
		}
	}
	// The integer an ID names is passed through FormatID.
	if id, err := IDToInt(FormatID(9000000001), "order_id"); err != nil || id != 9000000001 {
		t.Fatalf("IDToInt(FormatID(9000000001)) = %d, %v", id, err)
	}
}
