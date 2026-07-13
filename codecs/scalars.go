package codecs

import (
	"math/big"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/mr-tron/base58"
)

const (
	PriceTickScale = 6
	Uint64Max      = ^uint64(0)
)

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

// FormatPriceTicks formats int6 ticks as a decimal string.
func FormatPriceTicks(ticks int64) string {
	neg := ticks < 0
	if neg {
		ticks = -ticks
	}
	digits := formatPadded(ticks, PriceTickScale+1)
	head := strings.TrimLeft(digits[:len(digits)-PriceTickScale], "0")
	if head == "" {
		head = "0"
	}
	tail := strings.TrimRight(digits[len(digits)-PriceTickScale:], "0")
	out := head
	if tail != "" {
		out += "." + tail
	}
	if neg {
		return "-" + out
	}
	return out
}

// ParseQtyScaled parses a decimal quantity at the given scale (strict; no rounding).
func ParseQtyScaled(raw string, scale int, fieldName string) (uint64, error) {
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
func FormatQtyScaled(qtyScaled int64, scale int) string {
	if scale <= 0 {
		return formatInt(qtyScaled)
	}
	neg := qtyScaled < 0
	if neg {
		qtyScaled = -qtyScaled
	}
	digits := formatPadded(qtyScaled, scale+1)
	head := strings.TrimLeft(digits[:len(digits)-scale], "0")
	if head == "" {
		head = "0"
	}
	tail := strings.TrimRight(digits[len(digits)-scale:], "0")
	out := head
	if tail != "" {
		out += "." + tail
	}
	if neg {
		return "-" + out
	}
	return out
}

// IDToInt parses base58 or decimal uint64 ids.
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
		return n.Uint64(), nil
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
		value = 1
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

func formatPadded(n int64, width int) string {
	s := formatInt(n)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

func formatInt(n int64) string {
	return new(big.Int).SetInt64(n).String()
}
