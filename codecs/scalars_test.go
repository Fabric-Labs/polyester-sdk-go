package codecs

import (
	"encoding/binary"
	"testing"

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
	got, err = IDToInt("99", "trigger_id")
	if err != nil || got != 99 {
		t.Fatalf("decimal: got %d err %v", got, err)
	}
	if _, err := IDToInt("not a trigger id", "trigger_id"); err == nil {
		t.Fatal("expected validation error")
	}
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
