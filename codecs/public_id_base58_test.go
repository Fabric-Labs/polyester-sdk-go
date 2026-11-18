package codecs

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// A string public ID is base58, exactly as the API's own parser reads it: a
// string of digits is read as base58 too, never as a decimal number, so one
// string names one ID. A character outside the base58 alphabet, "0" or any
// whitespace included, is refused rather than trimmed or read as decimal.
func TestStringIDsAreBase58Only(t *testing.T) {
	for value, want := range map[string]uint64{
		"23":          60,
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
		if FormatID(got) != value {
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

// A string ID is accepted only in canonical form, as FormatID writes it. A
// leading "1" is a base58 zero digit and adds nothing, so "123" would name
// the same ID as "23" under another spelling: it is refused, naming the
// field, so one ID has one spelling.
func TestStringIDsMustBeCanonical(t *testing.T) {
	if id, err := IDToInt("23", "order_id"); err != nil || id != 60 {
		t.Fatalf(`IDToInt("23") = %d, %v; want 60`, id, err)
	}
	for _, value := range []string{"123", "1123", "12", "111111111112", "1z"} {
		id, err := IDToInt(value, "order_id")
		if err == nil {
			t.Fatalf("IDToInt(%q) = %d, want a ValidationError: not canonical", value, id)
		}
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || !strings.Contains(validation.Msg, "order_id") || !strings.Contains(validation.Msg, "canonical") {
			t.Fatalf("IDToInt(%q): err = %T %v, want a ValidationError naming order_id and canonical form", value, err, err)
		}
		if id, err := ParseAccountID(value, "sub_account_id"); err == nil {
			t.Fatalf("ParseAccountID(%q) = %d, want a ValidationError", value, id)
		}
	}
}

// Every canonical ID parses back to its number, and the same ID with a
// leading "1" is refused.
func TestCanonicalIDsRoundTrip(t *testing.T) {
	var ids []uint64
	for id := uint64(1); id < 5000; id++ {
		ids = append(ids, id)
	}
	pow := uint64(1)
	for i := 0; i < 10; i++ {
		pow *= 58
		ids = append(ids, pow)
	}
	for shift := 1; shift < 64; shift++ {
		ids = append(ids, uint64(1)<<shift, (uint64(1)<<shift)-1)
	}
	ids = append(ids, Uint64Max-1, Uint64Max)
	for _, id := range ids {
		text := FormatID(id)
		got, err := IDToInt(text, "order_id")
		if err != nil || got != id {
			t.Fatalf("IDToInt(%q) = %d, %v; want %d", text, got, err, id)
		}
		if got, err := IDToInt(fmt.Sprintf("1%s", text), "order_id"); err == nil {
			t.Fatalf("IDToInt(%q) = %d, want a ValidationError", "1"+text, got)
		}
	}
}
