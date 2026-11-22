package ethaddr

import (
	"strings"
	"testing"
)

// eip55Vectors are the test cases of the EIP-55 specification.
var eip55Vectors = []string{
	// All caps
	"0x52908400098527886E0F7030069857D2E4169EE7",
	"0x8617E340B3D01FA5F11F306F4090FD50E238070D",
	// All lower
	"0xde709f2102306220921060314715629080e2fb77",
	"0x27b1fdb04752bbc536007a920d24acb045561c26",
	// Normal
	"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
	"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
	"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
	"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
}

func TestChecksumMatchesEIP55Vectors(t *testing.T) {
	for _, want := range eip55Vectors {
		body := want[2:]
		for _, in := range []string{
			want,
			strings.ToLower(want),
			"0x" + strings.ToUpper(body),
			"0X" + body,
			body,
		} {
			got, ok := Checksum(in)
			if !ok || got != want {
				t.Errorf("Checksum(%q) = %q, %v; want %q", in, got, ok, want)
			}
		}
	}
}

func TestIsHex(t *testing.T) {
	valid := []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0X5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED",
		"5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
		"0x0000000000000000000000000000000000000000",
		// A wrong mixed-case checksum is still an address, as in go-ethereum.
		"0x5AAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
	}
	for _, s := range valid {
		if !IsHex(s) {
			t.Errorf("IsHex(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"",
		"0x",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAe",   // 39 digits
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed0", // 41 digits
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeg",  // non-hex digit
		" 0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed ",
		"x05aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0x0x5aAeb6053F3E94C9b9A09f33669435E7Ef1Be",
	}
	for _, s := range invalid {
		if IsHex(s) {
			t.Errorf("IsHex(%q) = true, want false", s)
		}
		if got, ok := Checksum(s); ok || got != "" {
			t.Errorf("Checksum(%q) = %q, %v; want \"\", false", s, got, ok)
		}
	}
}
