package codecs

import "testing"

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
