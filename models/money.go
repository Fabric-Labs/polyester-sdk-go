package models

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// QuantityDomain distinguishes order qty from asset/ledger amounts.
type QuantityDomain string

const (
	QuantityDomainOrderBase QuantityDomain = "order_base"
	QuantityDomainAsset     QuantityDomain = "asset"
	QuantityDomainLedgerE18 QuantityDomain = "ledger_e18"
)

const priceTickScale = 6

// PriceTicks is a resolved protocol price (protobuf price_ticks, fixed 1e6).
// Ticks are not market tick-size alignment; the server still validates tick size.
type PriceTicks struct {
	Ticks  int64  `json:"ticks"`
	Symbol string `json:"symbol,omitempty"`
}

// NewPriceTicks constructs a PriceTicks from wire units.
func NewPriceTicks(ticks int64) (PriceTicks, error) {
	if ticks < 0 {
		return PriceTicks{}, &errors.ValidationError{Msg: "ticks must be non-negative"}
	}
	return PriceTicks{Ticks: ticks}, nil
}

// MustPriceTicks is NewPriceTicks that panics on error (tests/examples).
func MustPriceTicks(ticks int64) PriceTicks {
	p, err := NewPriceTicks(ticks)
	if err != nil {
		panic(err)
	}
	return p
}

// Format returns the decimal string for this price.
func (p PriceTicks) Format() string {
	out, err := formatFixed(p.Ticks, priceTickScale)
	if err != nil {
		return "0"
	}
	return out
}

// CompatibleWith rejects known symbol mismatches.
func (p PriceTicks) CompatibleWith(symbol string) error {
	if p.Symbol != "" && symbol != "" && p.Symbol != symbol {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("price symbol mismatch: value is for %s, destination is %s", p.Symbol, symbol),
		}
	}
	return nil
}

// QtyScaled is a resolved order/trigger base quantity (protobuf qty_scaled).
type QtyScaled struct {
	Scaled   int64          `json:"scaled"`
	Scale    *int           `json:"scale,omitempty"`
	Domain   QuantityDomain `json:"domain,omitempty"`
	Symbol   string         `json:"symbol,omitempty"`
	SymbolID *uint32        `json:"symbol_id,omitempty"`
}

// NewQtyScaled constructs an order-base quantity from wire units.
func NewQtyScaled(scaled int64) (QtyScaled, error) {
	if scaled < 0 {
		return QtyScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return QtyScaled{Scaled: scaled, Domain: QuantityDomainOrderBase}, nil
}

// MustQtyScaled is NewQtyScaled that panics on error.
func MustQtyScaled(scaled int64) QtyScaled {
	q, err := NewQtyScaled(scaled)
	if err != nil {
		panic(err)
	}
	return q
}

// WithScale attaches formatting/compatibility scale metadata.
func (q QtyScaled) WithScale(scale int) QtyScaled {
	q.Scale = &scale
	return q
}

// WithSymbol attaches instrument metadata for compatibility checks.
func (q QtyScaled) WithSymbol(symbol string) QtyScaled {
	q.Symbol = symbol
	return q
}

// Format returns the decimal string; requires a known scale within MaxProtocolScale.
func (q QtyScaled) Format() (string, error) {
	if q.Scale == nil {
		return "", &errors.ValidationError{Msg: "format requires a known scale"}
	}
	return formatFixed(q.Scaled, *q.Scale)
}

// CompatibleWith rejects known domain/scale/instrument mismatches.
func (q QtyScaled) CompatibleWith(domain QuantityDomain, scale *int, symbol string, symbolID *uint32) error {
	if q.Domain == "" {
		q.Domain = QuantityDomainOrderBase
	}
	if domain == "" {
		domain = QuantityDomainOrderBase
	}
	if q.Domain != domain {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity domain mismatch: value is %s, destination is %s", q.Domain, domain),
		}
	}
	if q.Scale != nil && scale != nil && *q.Scale != *scale {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity scale mismatch: value scale is %d, destination is %d", *q.Scale, *scale),
		}
	}
	if q.Symbol != "" && symbol != "" && q.Symbol != symbol {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity symbol mismatch: value is for %s, destination is %s", q.Symbol, symbol),
		}
	}
	if q.SymbolID != nil && symbolID != nil && *q.SymbolID != *symbolID {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity symbol_id mismatch: value is for %d, destination is %d", *q.SymbolID, *symbolID),
		}
	}
	return nil
}

// AssetAmountScaled is a resolved transfer/withdraw amount.
type AssetAmountScaled struct {
	Scaled  *big.Int       `json:"scaled"`
	Scale   *int           `json:"scale,omitempty"`
	Domain  QuantityDomain `json:"domain,omitempty"`
	AssetID *uint32        `json:"asset_id,omitempty"`
}

// NewAssetAmountScaled constructs an asset amount from wire units (int64-safe).
func NewAssetAmountScaled(scaled int64) (AssetAmountScaled, error) {
	if scaled < 0 {
		return AssetAmountScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return AssetAmountScaled{
		Scaled: big.NewInt(scaled),
		Domain: QuantityDomainAsset,
	}, nil
}

// NewAssetAmountFromBig constructs an amount that may exceed int64 (e.g. e18).
func NewAssetAmountFromBig(scaled *big.Int) (AssetAmountScaled, error) {
	if scaled == nil || scaled.Sign() < 0 {
		return AssetAmountScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return AssetAmountScaled{
		Scaled: new(big.Int).Set(scaled),
		Domain: QuantityDomainLedgerE18,
	}, nil
}

// MustAssetAmountScaled panics on error.
func MustAssetAmountScaled(scaled int64) AssetAmountScaled {
	a, err := NewAssetAmountScaled(scaled)
	if err != nil {
		panic(err)
	}
	return a
}

// WithScale attaches formatting/compatibility scale.
func (a AssetAmountScaled) WithScale(scale int) AssetAmountScaled {
	a.Scale = &scale
	return a
}

// WithDomain sets asset vs ledger_e18 domain.
func (a AssetAmountScaled) WithDomain(domain QuantityDomain) AssetAmountScaled {
	a.Domain = domain
	return a
}

// WithAssetID attaches asset metadata for compatibility checks.
func (a AssetAmountScaled) WithAssetID(assetID uint32) AssetAmountScaled {
	a.AssetID = &assetID
	return a
}

// Int64 returns the scaled value when it fits in int64.
func (a AssetAmountScaled) Int64() (int64, error) {
	if a.Scaled == nil {
		return 0, &errors.ValidationError{Msg: "scaled is nil"}
	}
	if !a.Scaled.IsInt64() {
		return 0, &errors.ValidationError{Msg: "scaled exceeds int64 range"}
	}
	return a.Scaled.Int64(), nil
}

// Format returns the decimal string; requires a known scale within MaxProtocolScale.
func (a AssetAmountScaled) Format() (string, error) {
	if a.Scale == nil {
		return "", &errors.ValidationError{Msg: "format requires a known scale"}
	}
	if a.Scaled == nil {
		return "0", nil
	}
	return formatFixedBig(a.Scaled, *a.Scale)
}

// CompatibleWith rejects known domain/scale/asset mismatches.
func (a AssetAmountScaled) CompatibleWith(domain QuantityDomain, scale *int, assetID *uint32) error {
	if a.Domain == "" {
		a.Domain = QuantityDomainAsset
	}
	if domain == "" {
		domain = QuantityDomainAsset
	}
	if a.Domain != domain {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount domain mismatch: value is %s, destination is %s", a.Domain, domain),
		}
	}
	if a.Scale != nil && scale != nil && *a.Scale != *scale {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount scale mismatch: value scale is %d, destination is %d", *a.Scale, *scale),
		}
	}
	if a.AssetID != nil && assetID != nil && *a.AssetID != *assetID {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount asset_id mismatch: value is for %d, destination is %d", *a.AssetID, *assetID),
		}
	}
	return nil
}

// PriceInput is a write-side price: decimal string or PriceTicks.
type PriceInput struct {
	decimal  string
	hasDec   bool
	ticks    PriceTicks
	hasTicks bool
}

// PriceFromDecimal builds a human-path price input.
func PriceFromDecimal(s string) PriceInput {
	return PriceInput{decimal: s, hasDec: true}
}

// PriceFromTicks builds a bot-path price input.
func PriceFromTicks(p PriceTicks) PriceInput {
	return PriceInput{ticks: p, hasTicks: true}
}

// PriceFromTicksInt is PriceFromTicks(MustPriceTicks(n)).
func PriceFromTicksInt(ticks int64) PriceInput {
	return PriceFromTicks(MustPriceTicks(ticks))
}

// IsSet reports whether any path was provided.
func (p PriceInput) IsSet() bool { return p.hasDec || p.hasTicks }

// QtyInput is a write-side order/trigger quantity.
type QtyInput struct {
	decimal   string
	hasDec    bool
	scaled    QtyScaled
	hasScaled bool
}

// QtyFromDecimal builds a human-path quantity input.
func QtyFromDecimal(s string) QtyInput {
	return QtyInput{decimal: s, hasDec: true}
}

// QtyFromScaled builds a bot-path quantity input.
func QtyFromScaled(q QtyScaled) QtyInput {
	return QtyInput{scaled: q, hasScaled: true}
}

// QtyFromScaledInt is QtyFromScaled(MustQtyScaled(n)).
func QtyFromScaledInt(scaled int64) QtyInput {
	return QtyFromScaled(MustQtyScaled(scaled))
}

// IsSet reports whether any path was provided.
func (q QtyInput) IsSet() bool { return q.hasDec || q.hasScaled }

// AssetAmountInput is a write-side transfer/withdraw amount.
type AssetAmountInput struct {
	decimal   string
	hasDec    bool
	scaled    AssetAmountScaled
	hasScaled bool
}

// AssetAmountFromDecimal builds a human-path amount input.
func AssetAmountFromDecimal(s string) AssetAmountInput {
	return AssetAmountInput{decimal: s, hasDec: true}
}

// AssetAmountFromScaled builds a bot-path amount input.
func AssetAmountFromScaled(a AssetAmountScaled) AssetAmountInput {
	return AssetAmountInput{scaled: a, hasScaled: true}
}

// IsSet reports whether any path was provided.
func (a AssetAmountInput) IsSet() bool { return a.hasDec || a.hasScaled }

// Exported accessors for codecs (same package boundary).
func (p PriceInput) Decimal() (string, bool) { return p.decimal, p.hasDec }
func (p PriceInput) TicksValue() (PriceTicks, bool) {
	return p.ticks, p.hasTicks
}
func (q QtyInput) Decimal() (string, bool) { return q.decimal, q.hasDec }
func (q QtyInput) ScaledValue() (QtyScaled, bool) {
	return q.scaled, q.hasScaled
}
func (a AssetAmountInput) Decimal() (string, bool) { return a.decimal, a.hasDec }
func (a AssetAmountInput) ScaledValue() (AssetAmountScaled, bool) {
	return a.scaled, a.hasScaled
}

// MaxProtocolScale mirrors codecs.MaxProtocolScale for money formatters.
const MaxProtocolScale = 36

func formatFixed(n int64, scale int) (string, error) {
	if scale < 0 {
		return "", &errors.ValidationError{Msg: "scale must be non-negative"}
	}
	if scale > MaxProtocolScale {
		return "", &errors.ValidationError{
			Msg: fmt.Sprintf("scale %d exceeds maximum protocol scale %d", scale, MaxProtocolScale),
		}
	}
	if scale == 0 {
		return fmt.Sprintf("%d", n), nil
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := fmt.Sprintf("%d", n)
	width := scale + 1
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}
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
		return "-" + out, nil
	}
	return out, nil
}

func formatFixedBig(n *big.Int, scale int) (string, error) {
	if scale < 0 {
		return "", &errors.ValidationError{Msg: "scale must be non-negative"}
	}
	if scale > MaxProtocolScale {
		return "", &errors.ValidationError{
			Msg: fmt.Sprintf("scale %d exceeds maximum protocol scale %d", scale, MaxProtocolScale),
		}
	}
	if scale == 0 {
		return n.String(), nil
	}
	neg := n.Sign() < 0
	abs := new(big.Int).Abs(n)
	digits := abs.String()
	width := scale + 1
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}
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
		return "-" + out, nil
	}
	return out, nil
}

// Int64Max is math.MaxInt64 for bounds docs/tests.
const Int64Max = math.MaxInt64
