package catalogs

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// maxProtocolScale mirrors codecs.MaxProtocolScale without importing codecs
// (codecs → catalogs → codecs would cycle).
const maxProtocolScale = 36

// Manager holds hydrated spot and zipper catalogs.
// All public methods are safe for concurrent use.
type Manager struct {
	mu                    sync.RWMutex
	SpotConfig            map[string]any
	Zipper                *models.ZipperCatalogData
	DepositWithdrawConfig *models.DepositWithdrawConfig
	legacyZipperRaw       map[string]any
}

// NewManager creates an empty catalog manager.
func NewManager() *Manager {
	return &Manager{
		SpotConfig: map[string]any{},
	}
}

// ZipperConfig returns a backward-compatible raw dict view.
func (m *Manager) ZipperConfig() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.DepositWithdrawConfig == nil {
		return map[string]any{}
	}
	cfg := m.DepositWithdrawConfig
	return map[string]any{
		"chains":           cfg.Chains,
		"assets":           cfg.Assets,
		"contracts":        cfg.Contracts,
		"polyesterChainId": cfg.PolyesterChainID,
		"tsSec":            cfg.TsMs / 1000,
	}
}

// HydrateSpotConfig stores spot pair config after validating IDs and scales.
// Oversized u64 IDs/scales and scales above MaxProtocolScale are rejected
// (no silent truncation into uint32/int).
func (m *Manager) HydrateSpotConfig(config map[string]any) error {
	if err := validateSpotConfig(config); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SpotConfig = config
	return nil
}

// HydrateZipperConfig stores zipper catalog config.
// Typed configs are validated; legacy raw maps are stored only after scale/ID checks.
func (m *Manager) HydrateZipperConfig(config any) error {
	switch c := config.(type) {
	case models.DepositWithdrawConfig:
		return m.HydrateDepositWithdrawConfig(&c)
	case *models.DepositWithdrawConfig:
		return m.HydrateDepositWithdrawConfig(c)
	case map[string]any:
		if err := validateZipperRaw(c); err != nil {
			return err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.DepositWithdrawConfig = nil
		m.Zipper = nil
		m.legacyZipperRaw = c
		return nil
	default:
		return &errors.ValidationError{Msg: "unsupported zipper config type"}
	}
}

// HydrateDepositWithdrawConfig stores typed deposit/withdraw config.
func (m *Manager) HydrateDepositWithdrawConfig(config *models.DepositWithdrawConfig) error {
	if err := validateDepositWithdrawConfig(config); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DepositWithdrawConfig = config
	m.Zipper = BuildZipperCatalogData(config)
	m.legacyZipperRaw = nil
	return nil
}

// SymbolIDForSymbol resolves symbol id from spot config.
func (m *Manager) SymbolIDForSymbol(symbol string) *uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		if s, _ := pair["symbol"].(string); s == symbol {
			if v := intish(pair["symbol_id"]); v != nil {
				return v
			}
			if v := intish(pair["symbolId"]); v != nil {
				return v
			}
		}
	}
	return nil
}

// BaseQuantityScaleForSymbol returns qty scale for symbol.
//
// ok is false when the symbol is unknown or catalogs are unhydrated.
// Callers that need a decode-only fallback must choose it explicitly (never
// invent scale 8 here — that caused POLY-3549 false INSUFFICIENT_FUNDS).
func (m *Manager) BaseQuantityScaleForSymbol(symbol string) (scale int, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		if s, _ := pair["symbol"].(string); s == symbol {
			if v := intValue(pair["base_quantity_scale"]); v > 0 {
				return v, true
			}
			if v := intValue(pair["baseQuantityScale"]); v > 0 {
				return v, true
			}
			if v := intValue(pair["qtyScale"]); v > 0 {
				return v, true
			}
			return 0, false
		}
	}
	return 0, false
}

// BaseQuantityScaleForSymbolID returns qty scale for symbol id.
//
// ok is false when the id is unknown or catalogs are unhydrated.
func (m *Manager) BaseQuantityScaleForSymbolID(symbolID uint32) (scale int, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		v := intish(pair["symbol_id"])
		if v == nil {
			v = intish(pair["symbolId"])
		}
		if v != nil && *v == symbolID {
			if sym, _ := pair["symbol"].(string); sym != "" {
				for _, p := range m.pairsLocked() {
					if s, _ := p["symbol"].(string); s == sym {
						if scale := intValue(p["base_quantity_scale"]); scale > 0 {
							return scale, true
						}
						if scale := intValue(p["baseQuantityScale"]); scale > 0 {
							return scale, true
						}
						if scale := intValue(p["qtyScale"]); scale > 0 {
							return scale, true
						}
						return 0, false
					}
				}
			}
			break
		}
	}
	return 0, false
}

// OrderbookPriceBucketsForSymbol returns configured price buckets.
func (m *Manager) OrderbookPriceBucketsForSymbol(symbol string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pair := m.pairForSymbolLocked(symbol)
	if pair == nil {
		return nil
	}
	marketdata, _ := pair["marketdata"].(map[string]any)
	if marketdata == nil {
		marketdata, _ = pair["marketData"].(map[string]any)
	}
	if marketdata == nil {
		return nil
	}
	buckets, _ := marketdata["orderbook_price_buckets"].([]any)
	if buckets == nil {
		buckets, _ = marketdata["orderbookPriceBuckets"].([]any)
	}
	out := make([]string, 0, len(buckets))
	for _, value := range buckets {
		switch t := value.(type) {
		case float64:
			out = append(out, trimFloat(t))
		case int:
			out = append(out, strconv.Itoa(t))
		case string:
			out = append(out, t)
		}
	}
	return out
}

// LedgerIDForAsset resolves ledger id for asset symbol.
func (m *Manager) LedgerIDForAsset(assetSymbol string) *uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Zipper != nil {
		for _, asset := range m.Zipper.Assets {
			if asset.Asset == assetSymbol && asset.LedgerID != 0 {
				v := asset.LedgerID
				return &v
			}
		}
	}
	raw := m.legacyZipperRaw
	if raw == nil {
		if m.DepositWithdrawConfig == nil {
			return nil
		}
		cfg := m.DepositWithdrawConfig
		raw = map[string]any{
			"assets": cfg.Assets,
		}
	}
	assets, _ := raw["assets"].([]any)
	for _, row := range assets {
		item, _ := row.(map[string]any)
		symbol, _ := item["asset"].(string)
		if symbol == "" {
			symbol, _ = item["code"].(string)
		}
		if symbol == assetSymbol {
			return intish(item["ledgerId"])
		}
	}
	return nil
}

// QuantityScaleForAsset returns quantity scale for asset symbol.
func (m *Manager) QuantityScaleForAsset(assetSymbol string) *int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Zipper != nil {
		for _, asset := range m.Zipper.Assets {
			if asset.Asset == assetSymbol {
				v := asset.QuantityScale
				return &v
			}
		}
	}
	raw := m.legacyZipperRaw
	if raw == nil {
		if m.DepositWithdrawConfig == nil {
			return nil
		}
		raw = map[string]any{"assets": m.DepositWithdrawConfig.Assets}
	}
	assets, _ := raw["assets"].([]any)
	for _, row := range assets {
		item, _ := row.(map[string]any)
		symbol, _ := item["asset"].(string)
		if symbol == "" {
			symbol, _ = item["code"].(string)
		}
		if symbol == assetSymbol {
			if v := intValue(item["quantityScale"]); v > 0 {
				return &v
			}
			if v := intValue(item["quantity_scale"]); v > 0 {
				return &v
			}
		}
	}
	return nil
}

// QuantityScaleForZippedAssetID returns scale for zipped asset id.
func (m *Manager) QuantityScaleForZippedAssetID(zippedAssetID uint32) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Zipper != nil {
		for _, asset := range m.Zipper.Assets {
			for _, chain := range asset.Chains {
				if chain.ZippedAssetID == zippedAssetID {
					return asset.QuantityScale
				}
			}
		}
	}
	return 18
}

// PatchZipperSupply applies supply updates to the in-memory catalog.
func (m *Manager) PatchZipperSupply(updates []models.ZippedAssetSupplyUpdate) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Zipper == nil || len(updates) == 0 {
		return false
	}
	patched := PatchZipperCatalogSupply(m.Zipper, updates)
	if patched == m.Zipper {
		return false
	}
	m.Zipper = patched
	return true
}

func (m *Manager) pairForSymbolLocked(symbol string) map[string]any {
	for _, pair := range m.pairsLocked() {
		if s, _ := pair["symbol"].(string); s == symbol {
			return pair
		}
	}
	return nil
}

func (m *Manager) pairsLocked() []map[string]any {
	var pairs []map[string]any
	if raw, ok := m.SpotConfig["pairs"].([]any); ok {
		for _, item := range raw {
			if pair, ok := item.(map[string]any); ok {
				pairs = append(pairs, pair)
			}
		}
	}
	if raw, ok := m.SpotConfig["symbols"].([]any); ok {
		for _, item := range raw {
			if pair, ok := item.(map[string]any); ok {
				pairs = append(pairs, pair)
			}
		}
	}
	return pairs
}

func validateSpotConfig(config map[string]any) error {
	if config == nil {
		return nil
	}
	for _, key := range []string{"pairs", "symbols"} {
		raw, _ := config[key].([]any)
		for i, item := range raw {
			pair, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := validateOptionalU32Field(pair, "symbol_id", "symbolId"); err != nil {
				return fmt.Errorf("spot %s[%d]: %w", key, i, err)
			}
			if err := validateOptionalScaleField(pair, "base_quantity_scale", "baseQuantityScale", "qtyScale"); err != nil {
				return fmt.Errorf("spot %s[%d]: %w", key, i, err)
			}
		}
	}
	return nil
}

func validateZipperRaw(config map[string]any) error {
	if config == nil {
		return nil
	}
	assets, _ := config["assets"].([]any)
	for i, row := range assets {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if err := validateOptionalU32Field(item, "ledger_id", "ledgerId"); err != nil {
			return fmt.Errorf("zipper assets[%d]: %w", i, err)
		}
		if err := validateOptionalScaleField(item, "quantity_scale", "quantityScale"); err != nil {
			return fmt.Errorf("zipper assets[%d]: %w", i, err)
		}
	}
	return nil
}

func validateDepositWithdrawConfig(config *models.DepositWithdrawConfig) error {
	if config == nil {
		return nil
	}
	for i, asset := range config.Assets {
		if asset.QuantityScale < 0 || asset.QuantityScale > maxProtocolScale {
			return &errors.ValidationError{
				Msg: fmt.Sprintf("zipper assets[%d] quantity_scale %d exceeds maximum protocol scale %d", i, asset.QuantityScale, maxProtocolScale),
			}
		}
	}
	return nil
}

func validateOptionalU32Field(row map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, ok := row[key]; !ok {
			continue
		}
		if _, err := parseU32Exact(row[key], key); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func validateOptionalScaleField(row map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, ok := row[key]; !ok {
			continue
		}
		scale, err := parseIntExact(row[key], key)
		if err != nil {
			return err
		}
		if scale < 0 {
			return &errors.ValidationError{Msg: "scale must be non-negative"}
		}
		if scale > maxProtocolScale {
			return &errors.ValidationError{
				Msg: fmt.Sprintf("scale %d exceeds maximum protocol scale %d", scale, maxProtocolScale),
			}
		}
		return nil
	}
	return nil
}

func parseU32Exact(v any, field string) (uint32, error) {
	n, err := parseUintExact(v, field)
	if err != nil {
		return 0, err
	}
	if n > math.MaxUint32 {
		return 0, &errors.ValidationError{
			Msg: fmt.Sprintf("%s %d exceeds uint32 range", field, n),
		}
	}
	return uint32(n), nil
}

func parseUintExact(v any, field string) (uint64, error) {
	switch t := v.(type) {
	case float64:
		if t < 0 || t != math.Trunc(t) {
			return 0, &errors.ValidationError{Msg: field + " must be a non-negative integer"}
		}
		if t > float64(math.MaxUint64) {
			return 0, &errors.ValidationError{Msg: field + " exceeds uint64 range"}
		}
		// Reject values that cannot be represented exactly in float64 (and thus
		// would silently truncate when cast). JSON numbers above 2^53 are unsafe.
		if t > float64(1<<53) && t != float64(uint64(t)) {
			return 0, &errors.ValidationError{Msg: field + " is not an exact integer"}
		}
		u := uint64(t)
		if float64(u) != t {
			return 0, &errors.ValidationError{Msg: field + " exceeds exact integer range"}
		}
		return u, nil
	case int:
		if t < 0 {
			return 0, &errors.ValidationError{Msg: field + " must be a non-negative integer"}
		}
		return uint64(t), nil
	case int64:
		if t < 0 {
			return 0, &errors.ValidationError{Msg: field + " must be a non-negative integer"}
		}
		return uint64(t), nil
	case uint64:
		return t, nil
	case uint32:
		return uint64(t), nil
	case string:
		n, err := strconv.ParseUint(t, 10, 64)
		if err != nil {
			return 0, &errors.ValidationError{Msg: field + " must be a non-negative integer"}
		}
		return n, nil
	default:
		return 0, &errors.ValidationError{Msg: field + " must be a non-negative integer"}
	}
}

func parseIntExact(v any, field string) (int, error) {
	u, err := parseUintExact(v, field)
	if err != nil {
		return 0, err
	}
	if u > uint64(math.MaxInt) {
		return 0, &errors.ValidationError{Msg: field + " exceeds int range"}
	}
	return int(u), nil
}

func intish(v any) *uint32 {
	u, err := parseU32Exact(v, "id")
	if err != nil {
		return nil
	}
	return &u
}

func intValue(v any) int {
	n, err := parseIntExact(v, "scale")
	if err != nil {
		return 0
	}
	return n
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "0" {
		return "0"
	}
	return s
}
