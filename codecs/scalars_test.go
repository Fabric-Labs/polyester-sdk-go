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

	// FormatID(0) maps 0→1 then encodes; round-trip must stay consistent with
	// preferring the canonical base58 form.
	encoded0 := FormatID(0)
	got0, err := IDToInt(encoded0, "id")
	if err != nil {
		t.Fatal(err)
	}
	if FormatID(got0) != encoded0 {
		t.Fatalf("FormatID(0) round-trip inconsistent: decoded=%d FormatID=%q want %q", got0, FormatID(got0), encoded0)
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
		if FormatID(got) != encoded {
			t.Fatalf("id=%d encoded=%q: IDToInt=%d FormatID=%q", id, encoded, got, FormatID(got))
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

func TestParseAndFormatQtyScaled(t *testing.T) {
	qty, err := ParseQtyScaled("1.5", 8, "qty")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatQtyScaled(int64(qty), 8); got != "1.5" {
		t.Fatalf("got %q", got)
	}
}
