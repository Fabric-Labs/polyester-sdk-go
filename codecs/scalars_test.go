package codecs

import (
	"encoding/binary"
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/mr-tron/base58"
)

func TestIDToIntAcceptsBase58AndDecimal(t *testing.T) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], 42)
	encoded := base58.Encode(buf[:])
	got, err := IDToInt(encoded, "trigger_id")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("got %d", got)
	}
	got, err = IDToInt("100", "trigger_id")
	if err != nil || got != 100 {
		t.Fatalf("decimal: got %d err %v", got, err)
	}
	if _, err := IDToInt("not a trigger id", "trigger_id"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestIDToIntRoundTripFormatIDCanonical(t *testing.T) {
	// FormatID(4) == "5", but decimal parsing of "5" must not win over the
	// canonical base58 encoding of 4.
	if got := FormatID(4); got != "5" {
		t.Fatalf("FormatID(4)=%q want 5", got)
	}
	got, err := IDToInt(FormatID(4), "id")
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Fatalf("IDToInt(FormatID(4))=%d want 4", got)
	}

	// Zero has its own canonical base58 encoding and must not alias id 1.
	if got := FormatID(0); got != "1" {
		t.Fatalf("FormatID(0)=%q want 1", got)
	}
	if got := FormatID(1); got != "2" {
		t.Fatalf("FormatID(1)=%q want 2", got)
	}
	if FormatID(0) == FormatID(1) {
		t.Fatal("FormatID(0) must not alias FormatID(1)")
	}
}

func TestIDToIntPrefersCanonicalBase58ForAllDigitCollisions(t *testing.T) {
	for id := uint64(0); id <= 70; id++ {
		encoded := FormatID(id)
		allDigit := true
		for _, r := range encoded {
			if r < '0' || r > '9' {
				allDigit = false
				break
			}
		}
		if !allDigit {
			continue
		}
		got, err := IDToInt(encoded, "id")
		if err != nil {
			t.Fatalf("id=%d encoded=%q: %v", id, encoded, err)
		}
		if got != id {
			t.Fatalf("id=%d encoded=%q: IDToInt=%d", id, encoded, got)
		}
	}
}

func TestIDToIntNonCanonicalPureDecimalsStillWork(t *testing.T) {
	// Strings with '0' (not in base58) or non-canonical digit sequences stay decimal.
	for _, raw := range []string{"10", "100", "123", "123456789"} {
		got, err := IDToInt(raw, "id")
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		want := mustParseUint(raw)
		if got != want {
			t.Fatalf("%q: got %d want %d", raw, got, want)
		}
		if FormatID(got) == raw {
			t.Fatalf("%q unexpectedly canonical for %d", raw, got)
		}
	}
}

func TestIDToIntInvalidStillErrors(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a trigger id", "abc0OIl"} {
		_, err := IDToInt(raw, "trigger_id")
		if err == nil {
			t.Fatalf("%q: expected error", raw)
		}
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%q: want ValidationError, got %T %v", raw, err, err)
		}
	}
}

func mustParseUint(s string) uint64 {
	var n uint64
	for _, r := range s {
		n = n*10 + uint64(r-'0')
	}
	return n
}

func TestParseAndFormatPriceTicks(t *testing.T) {
	ticks, err := ParsePriceTicks("1.25", "price")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatPriceTicks(int64(ticks)); got != "1.25" {
		t.Fatalf("got %q", got)
	}
}

func TestParsePriceTicksRejectsTrailingDotAcceptsTrimmedWhitespace(t *testing.T) {
	for _, raw := range []string{"65000.", "65.", "."} {
		if _, err := ParsePriceTicks(raw, "price"); err == nil {
			t.Fatalf("%q: expected rejection", raw)
		}
	}
	// Leading/trailing whitespace is trimmed (TS value.trim() parity).
	for _, raw := range []string{" 65000", "65000 ", "65000.0"} {
		ticks, err := ParsePriceTicks(raw, "price")
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if ticks != 65_000_000_000 {
			t.Fatalf("%q: ticks=%d", raw, ticks)
		}
	}
}

func TestParseAndFormatQtyScaled(t *testing.T) {
	qty, err := ParseQtyScaled("1.5", 8, "qty")
	if err != nil {
		t.Fatal(err)
	}
	got, err := FormatQtyScaled(int64(qty), 8)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.5" {
		t.Fatalf("got %q", got)
	}
}

func TestParseQtyScaledAcceptsTrailingZerosBeyondScale(t *testing.T) {
	// POLY-4685: extra zeros past scale are padding, not extra precision.
	got, err := ParseQtyScaled("1.500000000", 8, "qty")
	if err != nil {
		t.Fatal(err)
	}
	want, err := ParseQtyScaled("1.5", 8, "qty")
	if err != nil {
		t.Fatal(err)
	}
	if got != want || got != 150_000_000 {
		t.Fatalf("got %d want 150000000", got)
	}
	if _, err := ParseQtyScaled("1.500000001", 8, "qty"); err == nil {
		t.Fatal("expected reject for non-zero extra digit")
	}
}
