package models

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// QuantityDomain distinguishes order qty from asset/ledger amounts.
type QuantityDomain string

const (
	QuantityDomainOrderBase  QuantityDomain = "order_base"
	QuantityDomainOrderQuote QuantityDomain = "order_quote"
	QuantityDomainAsset      QuantityDomain = "asset"
	QuantityDomainLedgerE18  QuantityDomain = "ledger_e18"
)

const priceTickScale = 6

// PriceTicks is a resolved protocol price (protobuf price_ticks, fixed 1e6).
// Ticks are not market tick-size alignment; the server still validates tick size.
type PriceTicks struct {
	ticks  int64
	symbol string
}

// NewPriceTicks constructs a PriceTicks from wire units.
func NewPriceTicks(ticks int64) (PriceTicks, error) {
	if ticks < 0 {
		return PriceTicks{}, &errors.ValidationError{Msg: "ticks must be non-negative"}
	}
	return PriceTicks{ticks: ticks}, nil
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
	out, err := formatFixed(p.ticks, priceTickScale)
	if err != nil {
		return "0"
	}
	return out
}

// CompatibleWith rejects known symbol mismatches.
func (p PriceTicks) CompatibleWith(symbol string) error {
	if p.symbol != "" && symbol != "" && p.symbol != symbol {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("price symbol mismatch: value is for %s, destination is %s", p.symbol, symbol),
		}
	}
	return nil
}

// Ticks returns the immutable protocol tick value.
func (p PriceTicks) Ticks() int64 { return p.ticks }

// Symbol returns the immutable optional symbol metadata.
func (p PriceTicks) Symbol() string { return p.symbol }

// WithSymbol returns a copy with symbol compatibility metadata.
func (p PriceTicks) WithSymbol(symbol string) PriceTicks {
	p.symbol = symbol
	return p
}

func (p PriceTicks) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Ticks  int64  `json:"ticks"`
		Symbol string `json:"symbol,omitempty"`
	}{p.ticks, p.symbol})
}

// QtyScaled is a resolved order/trigger base quantity (protobuf qty_scaled).
type QtyScaled struct {
	scaled   int64
	scale    *int
	domain   QuantityDomain
	symbol   string
	symbolID *uint32
}

// NewQtyScaled constructs an order-base quantity from wire units.
func NewQtyScaled(scaled int64) (QtyScaled, error) {
	if scaled < 0 {
		return QtyScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return QtyScaled{scaled: scaled, domain: QuantityDomainOrderBase}, nil
}

// NewQtyQuoteScaled constructs an order-quote quantity from wire units.
//
// scale is required so a bare integer can never silently inherit the catalog
// scale. Use catalogs.Manager.QuoteQuantityScaleForSymbol.
func NewQtyQuoteScaled(scaled int64, scale int) (QtyScaled, error) {
	if scaled < 0 {
		return QtyScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	if scale < 0 {
		return QtyScaled{}, &errors.ValidationError{Msg: "scale must be non-negative"}
	}
	if scale > MaxProtocolScale {
		return QtyScaled{}, &errors.ValidationError{
			Msg: fmt.Sprintf("scale %d exceeds maximum protocol scale %d", scale, MaxProtocolScale),
		}
	}
	return QtyScaled{scaled: scaled, scale: &scale, domain: QuantityDomainOrderQuote}, nil
}

// MustQtyQuoteScaled is NewQtyQuoteScaled that panics on error.
func MustQtyQuoteScaled(scaled int64, scale int) QtyScaled {
	q, err := NewQtyQuoteScaled(scaled, scale)
	if err != nil {
		panic(err)
	}
	return q
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
	q.scale = &scale
	return q
}

// WithSymbol attaches instrument metadata for compatibility checks.
func (q QtyScaled) WithSymbol(symbol string) QtyScaled {
	q.symbol = symbol
	return q
}

// WithSymbolID returns a copy with symbol ID compatibility metadata.
func (q QtyScaled) WithSymbolID(symbolID uint32) QtyScaled {
	q.symbolID = &symbolID
	return q
}

// WithDomain returns a copy with quantity-domain metadata.
func (q QtyScaled) WithDomain(domain QuantityDomain) QtyScaled {
	q.domain = domain
	return q
}

func (q QtyScaled) Scaled() int64          { return q.scaled }
func (q QtyScaled) Scale() *int            { return cloneInt(q.scale) }
func (q QtyScaled) Domain() QuantityDomain { return q.domain }
func (q QtyScaled) Symbol() string         { return q.symbol }
func (q QtyScaled) SymbolID() *uint32      { return cloneUint32(q.symbolID) }

func (q QtyScaled) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scaled   int64          `json:"scaled"`
		Scale    *int           `json:"scale,omitempty"`
		Domain   QuantityDomain `json:"domain,omitempty"`
		Symbol   string         `json:"symbol,omitempty"`
		SymbolID *uint32        `json:"symbol_id,omitempty"`
	}{q.scaled, q.scale, q.domain, q.symbol, q.symbolID})
}

// Format returns the decimal string; requires a known scale within MaxProtocolScale.
func (q QtyScaled) Format() (string, error) {
	if q.scale == nil {
		return "", &errors.ValidationError{Msg: "format requires a known scale"}
	}
	return formatFixed(q.scaled, *q.scale)
}

// CompatibleWith rejects known domain/scale/instrument mismatches.
func (q QtyScaled) CompatibleWith(domain QuantityDomain, scale *int, symbol string, symbolID *uint32) error {
	if q.domain == "" {
		q.domain = QuantityDomainOrderBase
	}
	if domain == "" {
		domain = QuantityDomainOrderBase
	}
	if q.domain != domain {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity domain mismatch: value is %s, destination is %s", q.domain, domain),
		}
	}
	if q.scale != nil && scale != nil && *q.scale != *scale {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity scale mismatch: value scale is %d, destination is %d", *q.scale, *scale),
		}
	}
	if q.symbol != "" && symbol != "" && q.symbol != symbol {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity symbol mismatch: value is for %s, destination is %s", q.symbol, symbol),
		}
	}
	if q.symbolID != nil && symbolID != nil && *q.symbolID != *symbolID {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("quantity symbol_id mismatch: value is for %d, destination is %d", *q.symbolID, *symbolID),
		}
	}
	return nil
}

// AssetAmountScaled is a resolved transfer/withdraw amount.
type AssetAmountScaled struct {
	scaled  *big.Int
	scale   *int
	domain  QuantityDomain
	assetID *uint32
}

// NewAssetAmountScaled constructs an asset amount from wire units (int64-safe).
func NewAssetAmountScaled(scaled int64) (AssetAmountScaled, error) {
	if scaled < 0 {
		return AssetAmountScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return AssetAmountScaled{
		scaled: big.NewInt(scaled),
		domain: QuantityDomainAsset,
	}, nil
}

// NewAssetAmountFromBig constructs an amount that may exceed int64 (e.g. e18).
func NewAssetAmountFromBig(scaled *big.Int) (AssetAmountScaled, error) {
	if scaled == nil || scaled.Sign() < 0 {
		return AssetAmountScaled{}, &errors.ValidationError{Msg: "scaled must be non-negative"}
	}
	return AssetAmountScaled{
		scaled: new(big.Int).Set(scaled),
		domain: QuantityDomainLedgerE18,
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
	a.scale = &scale
	return a
}

// WithDomain sets asset vs ledger_e18 domain.
func (a AssetAmountScaled) WithDomain(domain QuantityDomain) AssetAmountScaled {
	a.domain = domain
	return a
}

// WithAssetID attaches asset metadata for compatibility checks.
func (a AssetAmountScaled) WithAssetID(assetID uint32) AssetAmountScaled {
	a.assetID = &assetID
	return a
}

// Scaled returns a deep copy of the immutable scaled integer.
func (a AssetAmountScaled) Scaled() *big.Int {
	if a.scaled == nil {
		return nil
	}
	return new(big.Int).Set(a.scaled)
}
func (a AssetAmountScaled) Scale() *int            { return cloneInt(a.scale) }
func (a AssetAmountScaled) Domain() QuantityDomain { return a.domain }
func (a AssetAmountScaled) AssetID() *uint32       { return cloneUint32(a.assetID) }

func (a AssetAmountScaled) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scaled  *big.Int       `json:"scaled"`
		Scale   *int           `json:"scale,omitempty"`
		Domain  QuantityDomain `json:"domain,omitempty"`
		AssetID *uint32        `json:"asset_id,omitempty"`
	}{a.Scaled(), a.scale, a.domain, a.assetID})
}

// Int64 returns the scaled value when it fits in int64.
func (a AssetAmountScaled) Int64() (int64, error) {
	if a.scaled == nil {
		return 0, &errors.ValidationError{Msg: "scaled is nil"}
	}
	if !a.scaled.IsInt64() {
		return 0, &errors.ValidationError{Msg: "scaled exceeds int64 range"}
	}
	return a.scaled.Int64(), nil
}

// Format returns the decimal string; requires a known scale within MaxProtocolScale.
func (a AssetAmountScaled) Format() (string, error) {
	if a.scale == nil {
		return "", &errors.ValidationError{Msg: "format requires a known scale"}
	}
	if a.scaled == nil {
		return "0", nil
	}
	return formatFixedBig(a.scaled, *a.scale)
}

// CompatibleWith rejects known domain/scale/asset mismatches.
func (a AssetAmountScaled) CompatibleWith(domain QuantityDomain, scale *int, assetID *uint32) error {
	if a.domain == "" {
		a.domain = QuantityDomainAsset
	}
	if domain == "" {
		domain = QuantityDomainAsset
	}
	if a.domain != domain {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount domain mismatch: value is %s, destination is %s", a.domain, domain),
		}
	}
	if a.scale != nil && scale != nil && *a.scale != *scale {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount scale mismatch: value scale is %d, destination is %d", *a.scale, *scale),
		}
	}
	if a.assetID != nil && assetID != nil && *a.assetID != *assetID {
		return &errors.ValidationError{
			Msg: fmt.Sprintf("amount asset_id mismatch: value is for %d, destination is %d", *a.assetID, *assetID),
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

// QtyFromQuoteDecimal builds a human-path quote-debit budget input.
//
// Scale is applied at resolve time from the pair's catalog quote_quantity_scale.
func QtyFromQuoteDecimal(s string) QtyInput {
	return QtyFromDecimal(s)
}

// QtyFromQuoteScaled builds a bot-path quote-debit budget with OrderQuote domain.
func QtyFromQuoteScaled(scaled int64, scale int) QtyInput {
	return QtyFromScaled(MustQtyQuoteScaled(scaled, scale))
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
	return AssetAmountInput{scaled: a.clone(), hasScaled: true}
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
	return a.scaled.clone(), a.hasScaled
}

func (a AssetAmountScaled) clone() AssetAmountScaled {
	out := a
	out.scale = cloneInt(a.scale)
	out.assetID = cloneUint32(a.assetID)
	if a.scaled != nil {
		out.scaled = new(big.Int).Set(a.scaled)
	}
	return out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneUint32(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
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
