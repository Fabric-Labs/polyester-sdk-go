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
		return ticks.Ticks, nil
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
		if qty.Scaled <= 0 {
			return 0, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		return qty.Scaled, nil
	}
	if dec, ok := in.Decimal(); ok {
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

// ResolveAssetAmountScaled resolves an AssetAmountInput to a big.Int scaled value.
func ResolveAssetAmountScaled(
	in models.AssetAmountInput,
	scale int,
	fieldName string,
	domain models.QuantityDomain,
	assetID *uint32,
) (*big.Int, error) {
	if domain == "" {
		domain = models.QuantityDomainAsset
	}
	if amt, ok := in.ScaledValue(); ok {
		scalePtr := &scale
		if err := amt.CompatibleWith(domain, scalePtr, assetID); err != nil {
			return nil, err
		}
		if amt.Scaled == nil || amt.Scaled.Sign() <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		if domain != models.QuantityDomainLedgerE18 && !amt.Scaled.IsInt64() {
			return nil, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
		}
		return new(big.Int).Set(amt.Scaled), nil
	}
	if dec, ok := in.Decimal(); ok {
		scaled, err := decimalToScaledBig(dec, scale, fieldName)
		if err != nil {
			return nil, err
		}
		if scaled.Sign() <= 0 {
			return nil, &errors.ValidationError{Msg: fieldName + " must be positive"}
		}
		if domain != models.QuantityDomainLedgerE18 && !scaled.IsInt64() {
			return nil, &errors.ValidationError{Msg: fieldName + " exceeds int64 range"}
		}
		return scaled, nil
	}
	return nil, &errors.ValidationError{Msg: fieldName + " is required"}
}

// DecodePriceTicks builds a read-side PriceTicks.
func DecodePriceTicks(ticks int64, symbol string) models.PriceTicks {
	return models.PriceTicks{Ticks: ticks, Symbol: symbol}
}

// DecodeQtyScaled builds a read-side order quantity.
func DecodeQtyScaled(scaled int64, scale int, symbol string, symbolID *uint32) models.QtyScaled {
	q := models.QtyScaled{
		Scaled:   scaled,
		Domain:   models.QuantityDomainOrderBase,
		Symbol:   symbol,
		SymbolID: symbolID,
	}
	if scale >= 0 {
		q.Scale = &scale
	}
	return q
}

// DecodeAssetAmount builds a read-side asset amount.
func DecodeAssetAmount(scaled int64, scale int, domain models.QuantityDomain, assetID *uint32) models.AssetAmountScaled {
	a := models.AssetAmountScaled{
		Scaled:  big.NewInt(scaled),
		Domain:  domain,
		AssetID: assetID,
	}
	if scale >= 0 {
		a.Scale = &scale
	}
	return a
}

func decimalToScaledBig(raw string, scale int, fieldName string) (*big.Int, error) {
	text, err := normalizeStrictDecimal(raw, fieldName)
	if err != nil {
		return nil, err
	}
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

func itoa(n int) string {
	return new(big.Int).SetInt64(int64(n)).String()
}

// Int64Bounds helpers used by money tests.
const (
	int64Max = math.MaxInt64
	int64Min = math.MinInt64
)
