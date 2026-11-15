package codecs

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func withdrawPayload(key, nonce string) (string, uint64, error) {
	deadline := uint64(1_800_000_000)
	payload, err := TradingWithdrawPayloadToProto(
		"to_funding", 2, models.AssetAmountFromDecimal("1"), key, 0, &deadline, nonce, "", 0,
	)
	if err != nil {
		return "", 0, err
	}
	return payload.GetIdempotencyKey(), payload.GetNonce().GetLo(), nil
}

// The idempotency key is signed exactly as given, never trimmed: the
// contract only requires it to be non-empty, so only "" is refused.
func TestTradingWithdrawSignsTheIdempotencyKeyAsGiven(t *testing.T) {
	if withdrawIdempotencyKeyMinLen != 1 {
		t.Fatalf("idempotency_key min_len = %d, want the contract's 1", withdrawIdempotencyKeyMinLen)
	}
	for _, key := range []string{" wd-x ", "wd-x", "  ", "\t", "é"} {
		got, _, err := withdrawPayload(key, "42")
		if err != nil {
			t.Fatalf("%q: %v", key, err)
		}
		if got != key {
			t.Fatalf("key sent %q, want %q", got, key)
		}
	}
	_, _, err := withdrawPayload("", "42")
	wantValidation(t, err, "idempotency_key")
}

// The nonce is a string of ASCII decimal digits, signed exactly; any other
// form, including surrounding whitespace or a sign, is refused rather than
// rewritten.
func TestTradingWithdrawNonceIsExactDecimalDigits(t *testing.T) {
	for _, nonce := range []string{"  42  ", " 42", "42 ", "\t42", "+42", "-42", "1.5", "42.0", "0x10", "4_2", "1e3", "٤٢", "", "0", "000"} {
		_, _, err := withdrawPayload("wd-x", nonce)
		wantValidation(t, err, "nonce")
	}
	for nonce, want := range map[string]uint64{"42": 42, "042": 42, "1": 1} {
		_, got, err := withdrawPayload("wd-x", nonce)
		if err != nil {
			t.Fatalf("%q: %v", nonce, err)
		}
		if got != want {
			t.Fatalf("nonce %q signed %d, want %d", nonce, got, want)
		}
	}
}
