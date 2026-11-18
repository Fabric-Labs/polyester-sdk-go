package codecs

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/mr-tron/base58"
)

const (
	PriceTickScale = models.PriceTickScale
	Uint64Max      = ^uint64(0)
	// MaxProtocolScale is the maximum accepted quantity/ledger scale for public
	// formatters, parsers, and catalog hydration. Values above this are rejected
	// instead of allocating pathological padding (scale ≥ 65535 historically panicked).
	MaxProtocolScale = 36
)

// ValidateProtocolScale rejects scales above MaxProtocolScale.
func ValidateProtocolScale(scale int) error {
	if scale < 0 {
		return &errors.ValidationError{Msg: "scale must be non-negative"}
	}
	if scale > MaxProtocolScale {
		return &errors.ValidationError{
			Msg: "scale " + itoa(scale) + " exceeds maximum protocol scale " + itoa(MaxProtocolScale),
		}
	}
	return nil
}

// ParsePriceTicks parses a decimal price string into int64 ticks (strict; no rounding).
func ParsePriceTicks(raw, fieldName string) (uint64, error) {
	scaled, err := decimalToScaledBig(raw, PriceTickScale, fieldName)
	if err != nil {
		return 0, err
	}
	if scaled.Sign() < 0 {
		return 0, &errors.ValidationError{Msg: fieldName + " must be non-negative"}
	}
	if !scaled.IsUint64() {
		return 0, &errors.ValidationError{Msg: fieldName + " out of range"}
	}
	u := scaled.Uint64()
	if u > uint64(int64Max) {
		return 0, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
	}
	return u, nil
}

// FormatPriceTicks formats Q9 price ticks as a decimal string.
func FormatPriceTicks(ticks int64) string {
	return formatScaledInt64(ticks, PriceTickScale)
}

// ParseQtyScaled parses a decimal quantity at the given scale (strict; no rounding).
func ParseQtyScaled(raw string, scale int, fieldName string) (uint64, error) {
	if err := ValidateProtocolScale(scale); err != nil {
		return 0, err
	}
	scaled, err := decimalToScaledBig(raw, scale, fieldName)
	if err != nil {
		return 0, err
	}
	if scaled.Sign() <= 0 {
		return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
	}
	if scale != 18 && !scaled.IsInt64() {
		return 0, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
	}
	if !scaled.IsUint64() {
		return 0, &errors.ValidationError{Msg: fieldName + " out of range"}
	}
	return scaled.Uint64(), nil
}

// FormatQtyScaled formats a scaled integer quantity.
// Scale must be in [0, MaxProtocolScale]; larger values return a ValidationError
// instead of allocating huge zero-pads.
func FormatQtyScaled(qtyScaled int64, scale int) (string, error) {
	if err := ValidateProtocolScale(scale); err != nil {
		return "", err
	}
	return formatScaledInt64(qtyScaled, scale), nil
}

// formatScaledInt64 formats n / 10^scale for a validated scale. The magnitude
// is taken as uint64 so math.MinInt64, whose negation overflows, keeps one sign.
func formatScaledInt64(n int64, scale int) string {
	magnitude := uint64(n)
	if n < 0 {
		magnitude = -magnitude
	}
	digits := strconv.FormatUint(magnitude, 10)
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale+1-len(digits)) + digits
		}
		head := digits[:len(digits)-scale]
		if tail := strings.TrimRight(digits[len(digits)-scale:], "0"); tail != "" {
			digits = head + "." + tail
		} else {
			digits = head
		}
	}
	if n < 0 {
		return "-" + digits
	}
	return digits
}

// IDToInt parses a base58 public ID, as the API reads one, and refuses one
// that decodes to 0. On the wire 0 is unset and names nothing, so "1"
// (FormatID(0)) would send a request for an ID the caller did not mean.
//
// A string ID is base58 only: a string of digits is read as base58 too, never
// as a decimal number. Only the canonical spelling, the one FormatID writes,
// is accepted: a leading "1" is a base58 zero digit, so "123" would name the
// same ID as "23", and it is refused. One ID has one spelling. Pass an ID the
// API returned as it is, and an integer through FormatID.
func IDToInt(value string, label string) (uint64, error) {
	id, err := parseID(value, label)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, &errors.ValidationError{Msg: label + " must be a non-zero public ID"}
	}
	if FormatID(id) != value {
		return 0, &errors.ValidationError{Msg: label + " must be a public ID in canonical form, as the SDK writes it (no leading 1)"}
	}
	return id, nil
}

// base58Digits maps each byte of the base58 alphabet to its digit value and
// every other byte to 0xFF.
var base58Digits = func() (table [256]byte) {
	for i := range table {
		table[i] = 0xFF
	}
	for i, c := range []byte(base58Alphabet) {
		table[c] = byte(i)
	}
	return table
}()

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// parseID reads a public ID exactly as the API does: every byte a base58
// digit, nothing trimmed, and a value that fits in a uint64.
func parseID(value string, label string) (uint64, error) {
	if value == "" {
		return 0, &errors.ValidationError{Msg: label + " is required"}
	}
	var id uint64
	for i := 0; i < len(value); i++ {
		digit := base58Digits[value[i]]
		if digit == 0xFF {
			return 0, &errors.ValidationError{Msg: label + " must be a base58 public ID, without spaces; pass an integer ID through FormatID"}
		}
		if id > (Uint64Max-uint64(digit))/58 {
			return 0, &errors.ValidationError{Msg: label + " exceeds uint64 range"}
		}
		id = id*58 + uint64(digit)
	}
	return id, nil
}

// ParseAccountID decodes an account or sub-account ID as IDToInt does,
// refusing one that decodes to 0. On the wire 0 is unset: a sub-account ID of
// 0 is the main account, and an account ID of 0 names no account. So "1"
// (FormatID(0)), "0" or "00" would address another account than the caller
// named. Omit a sub-account ID to address the main account.
func ParseAccountID(value string, label string) (uint64, error) {
	return IDToInt(value, label)
}

// FormatID encodes a uint64 id as base58.
func FormatID(value uint64) string {
	if value == 0 {
		return base58.Encode([]byte{0})
	}
	return base58.Encode(new(big.Int).SetUint64(value).Bytes())
}

func isDecimal(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
