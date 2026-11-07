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

// IDToInt parses base58 or decimal uint64 ids.
//
// All-digit strings are ambiguous: they may be a decimal literal or a base58
// encoding that happens to use only digit characters (e.g. FormatID(4) == "5").
// For those inputs, prefer the canonical base58 decode when FormatID(b) matches
// the input; otherwise treat the value as decimal.
func IDToInt(value string, label string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, &errors.ValidationError{Msg: label + " is required"}
	}
	if isDecimal(value) {
		n := new(big.Int)
		if _, ok := n.SetString(value, 10); !ok {
			return 0, &errors.ValidationError{Msg: label + " must be base58 or decimal uint64"}
		}
		if n.Sign() < 0 || n.Cmp(new(big.Int).SetUint64(Uint64Max)) > 0 {
			return 0, &errors.ValidationError{Msg: label + " exceeds uint64 range"}
		}
		d := n.Uint64()
		if decoded, err := base58.Decode(value); err == nil {
			b := new(big.Int).SetBytes(decoded)
			if b.Sign() >= 0 && b.Cmp(new(big.Int).SetUint64(Uint64Max)) <= 0 {
				canonical := b.Uint64()
				if FormatID(canonical) == value {
					return canonical, nil
				}
			}
		}
		return d, nil
	}
	decoded, err := base58.Decode(value)
	if err != nil {
		return 0, &errors.ValidationError{Msg: label + " must be base58 or decimal uint64"}
	}
	n := new(big.Int).SetBytes(decoded)
	if n.Sign() < 0 || n.Cmp(new(big.Int).SetUint64(Uint64Max)) > 0 {
		return 0, &errors.ValidationError{Msg: label + " exceeds uint64 range"}
	}
	return n.Uint64(), nil
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
