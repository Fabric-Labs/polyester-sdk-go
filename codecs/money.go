package codecs

import (
	"math"
	"math/big"
	"regexp"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var strictDecimal = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

// ResolvePriceTicks resolves a PriceInput to protocol ticks.
func ResolvePriceTicks(in models.PriceInput, fieldName, symbol string) (int64, error) {
	if ticks, ok := in.TicksValue(); ok {
		if err := ticks.CompatibleWith(symbol); err != nil {
			return 0, err
		}
		return ticks.Ticks(), nil
	}
	if dec, ok := in.Decimal(); ok {
		u, err := ParsePriceTicks(dec, fieldName)
		if err != nil {
			return 0, err
		}
		return int64(u), nil
	}
	return 0, &errors.ValidationError{Msg: fieldName + " is required"}
}

// ResolveOptionalPriceTicks resolves an optional price pointer.
func ResolveOptionalPriceTicks(in *models.PriceInput, fieldName, symbol string) (*int64, error) {
	if in == nil || !in.IsSet() {
		return nil, nil
	}
	ticks, err := ResolvePriceTicks(*in, fieldName, symbol)
	if err != nil {
		return nil, err
	}
	return &ticks, nil
}

// ResolveQtyScaled resolves a QtyInput to wire qty_scaled.
func ResolveQtyScaled(in models.QtyInput, scale int, fieldName, symbol string, symbolID *uint32) (int64, error) {
	if qty, ok := in.ScaledValue(); ok {
		scalePtr := &scale
		if err := qty.CompatibleWith(models.QuantityDomainOrderBase, scalePtr, symbol, symbolID); err != nil {
			return 0, err
		}
		if qty.Scaled() <= 0 {
			return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		return qty.Scaled(), nil
	}
	if dec, ok := in.Decimal(); ok {
		if scale <= 0 {
			return 0, &errors.ValidationError{
				Msg: fieldName + " requires known quantity scale (catalogs + symbol)",
			}
		}
		u, err := ParseQtyScaled(dec, scale, fieldName)
		if err != nil {
			return 0, err
		}
		return int64(u), nil
	}
	return 0, &errors.ValidationError{Msg: fieldName + " is required"}
}

// ResolveOptionalQtyScaled resolves an optional qty pointer.
func ResolveOptionalQtyScaled(in *models.QtyInput, scale int, fieldName, symbol string, symbolID *uint32) (*int64, error) {
	if in == nil || !in.IsSet() {
		return nil, nil
	}
	qty, err := ResolveQtyScaled(*in, scale, fieldName, symbol, symbolID)
	if err != nil {
		return nil, err
	}
	return &qty, nil
}

// ResolveQuoteQtyScaled resolves a quote-debit budget. Scaled inputs must carry
// OrderQuote domain and an explicit scale matching the catalog quote scale.
func ResolveQuoteQtyScaled(in models.QtyInput, scale int, fieldName, symbol string, symbolID *uint32) (int64, error) {
	if qty, ok := in.ScaledValue(); ok {
		if qty.Scale() == nil {
			return 0, &errors.ValidationError{
				Msg: fieldName + " scale is required; use QtyFromQuoteScaled/QtyFromQuoteDecimal",
			}
		}
		scalePtr := &scale
		if err := qty.CompatibleWith(models.QuantityDomainOrderQuote, scalePtr, symbol, symbolID); err != nil {
			return 0, err
		}
		if qty.Scaled() <= 0 {
			return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		return qty.Scaled(), nil
	}
	if dec, ok := in.Decimal(); ok {
		if scale < 0 {
			return 0, &errors.ValidationError{
				Msg: fieldName + " requires known quote quantity scale (catalogs + symbol)",
			}
		}
		u, err := ParseQtyScaled(dec, scale, fieldName)
		if err != nil {
			return 0, err
		}
		if int64(u) <= 0 {
			return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		return int64(u), nil
	}
	return 0, &errors.ValidationError{Msg: fieldName + " is required"}
}

// ResolveAssetAmountScaled resolves an AssetAmountInput to a big.Int scaled value.
func ResolveAssetAmountScaled(
	in models.AssetAmountInput,
	scale int,
	fieldName string,
	domain models.QuantityDomain,
	assetID *uint32,
) (*big.Int, error) {
	return ResolveAssetAmountScaledToScale(in, &scale, scale, fieldName, domain, assetID)
}

// ResolveAssetAmountScaledToScale resolves an amount from its declared/input
// scale to a fixed target scale exactly. It never rounds.
func ResolveAssetAmountScaledToScale(
	in models.AssetAmountInput,
	inputScale *int,
	targetScale int,
	fieldName string,
	domain models.QuantityDomain,
	assetID *uint32,
) (*big.Int, error) {
	if domain == "" {
		domain = models.QuantityDomainAsset
	}
	if err := ValidateProtocolScale(targetScale); err != nil {
		return nil, err
	}
	if amt, ok := in.ScaledValue(); ok {
		if err := amt.CompatibleWith(domain, nil, assetID); err != nil {
			return nil, err
		}
		value := amt.Scaled()
		if value == nil || value.Sign() <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		var sourceScale int
		if declared := amt.Scale(); declared != nil {
			sourceScale = *declared
			if inputScale != nil && *inputScale != sourceScale {
				return nil, &errors.ValidationError{Msg: fieldName + " scale metadata does not match input scale"}
			}
		} else if inputScale != nil {
			sourceScale = *inputScale
		} else {
			return nil, &errors.ValidationError{
				Msg: fieldName + " scale is required; construct AssetAmount with an explicit scale or pass amount_scale/quantity_scale",
			}
		}
		rescaled, err := rescaleExact(value, sourceScale, targetScale, fieldName)
		if err != nil {
			return nil, err
		}
		if domain != models.QuantityDomainLedgerE18 && !rescaled.IsInt64() {
			return nil, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
		}
		return rescaled, nil
	}
	if dec, ok := in.Decimal(); ok {
		sourceScale := targetScale
		if inputScale != nil {
			sourceScale = *inputScale
		}
		scaled, err := decimalToScaledBig(dec, sourceScale, fieldName)
		if err != nil {
			return nil, err
		}
		if scaled.Sign() <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		scaled, err = rescaleExact(scaled, sourceScale, targetScale, fieldName)
		if err != nil {
			return nil, err
		}
		if domain != models.QuantityDomainLedgerE18 && !scaled.IsInt64() {
			return nil, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
		}
		return scaled, nil
	}
	return nil, &errors.ValidationError{Msg: fieldName + " is required"}
}

func rescaleExact(value *big.Int, sourceScale, targetScale int, fieldName string) (*big.Int, error) {
	if err := ValidateProtocolScale(sourceScale); err != nil {
		return nil, err
	}
	if sourceScale == targetScale {
		if value.BitLen() > 128 {
			return nil, &errors.ValidationError{Msg: fieldName + " exceeds uint128 range"}
		}
		return new(big.Int).Set(value), nil
	}
	diff := sourceScale - targetScale
	if diff < 0 {
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-diff)), nil)
		out := new(big.Int).Mul(value, factor)
		if out.BitLen() > 128 {
			return nil, &errors.ValidationError{Msg: fieldName + " scale conversion exceeds uint128 range"}
		}
		return out, nil
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(diff)), nil)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value, divisor, remainder)
	if remainder.Sign() != 0 {
		return nil, &errors.ValidationError{Msg: fieldName + " cannot be represented exactly at target scale"}
	}
	if quotient.BitLen() > 128 {
		return nil, &errors.ValidationError{Msg: fieldName + " exceeds uint128 range"}
	}
	return quotient, nil
}

// DecodePriceTicks builds a read-side PriceTicks.
func DecodePriceTicks(ticks int64, symbol string) models.PriceTicks {
	value, err := models.NewPriceTicks(ticks)
	if err != nil {
		return models.PriceTicks{}
	}
	return value.WithSymbol(symbol)
}

// DecodeQtyScaled builds a read-side order quantity.
func DecodeQtyScaled(scaled int64, scale int, symbol string, symbolID *uint32) models.QtyScaled {
	q, _ := models.NewQtyScaled(scaled)
	if scale >= 0 {
		q = q.WithScale(scale)
	}
	q = q.WithSymbol(symbol)
	if symbolID != nil {
		q = q.WithSymbolID(*symbolID)
	}
	return q
}

// DecodeAssetAmount builds a read-side asset amount.
func DecodeAssetAmount(scaled int64, scale int, domain models.QuantityDomain, assetID *uint32) models.AssetAmountScaled {
	return DecodeAssetAmountBig(big.NewInt(scaled), scale, domain, assetID)
}

// DecodeAssetAmountBig builds a read-side asset amount from a big.Int (U128-safe).
func DecodeAssetAmountBig(scaled *big.Int, scale int, domain models.QuantityDomain, assetID *uint32) models.AssetAmountScaled {
	var value *big.Int
	if scaled != nil {
		value = new(big.Int).Set(scaled)
	} else {
		value = big.NewInt(0)
	}
	a, _ := models.NewAssetAmountFromBig(value)
	a = a.WithDomain(domain)
	if scale >= 0 {
		a = a.WithScale(scale)
	}
	if assetID != nil {
		a = a.WithAssetID(*assetID)
	}
	return a
}

func decimalToScaledBig(raw string, scale int, fieldName string) (*big.Int, error) {
	if err := ValidateProtocolScale(scale); err != nil {
		return nil, err
	}
	text, err := normalizeStrictDecimal(raw, fieldName)
	if err != nil {
		return nil, err
	}
	text = trimTrailingFracZeros(text)
	intPart, fracPart, _ := strings.Cut(text, ".")
	if len(fracPart) > scale {
		return nil, &errors.ValidationError{
			Msg: fieldName + " supports at most " + itoa(scale) + " decimal places: " + text,
		}
	}
	digits := intPart + fracPart + strings.Repeat("0", scale-len(fracPart))
	if digits == "" {
		digits = "0"
	}
	n := new(big.Int)
	if _, ok := n.SetString(digits, 10); !ok {
		return nil, &errors.ValidationError{Msg: fieldName + " must be a valid decimal string"}
	}
	return n, nil
}

func normalizeStrictDecimal(raw, fieldName string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !strictDecimal.MatchString(text) {
		return "", &errors.ValidationError{Msg: fieldName + " must be a valid decimal string"}
	}
	return text, nil
}

func trimTrailingFracZeros(raw string) string {
	intPart, fracPart, ok := strings.Cut(raw, ".")
	if !ok {
		return raw
	}
	fracPart = strings.TrimRight(fracPart, "0")
	if fracPart == "" {
		if intPart == "" {
			return "0"
		}
		return intPart
	}
	return intPart + "." + fracPart
}

func itoa(n int) string {
	return new(big.Int).SetInt64(int64(n)).String()
}

// Int64Bounds helpers used by money tests.
const (
	int64Max = math.MaxInt64
	int64Min = math.MinInt64
)
