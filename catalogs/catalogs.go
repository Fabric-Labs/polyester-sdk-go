package catalogs

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"sync"
	"time"

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
	clonedValue, err := cloneCatalogValue(config)
	if err != nil {
		return err
	}
	cloned := clonedValue.(map[string]any)
	if err := validateSpotConfig(cloned); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SpotConfig = cloned
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
		clonedValue, err := cloneCatalogValue(c)
		if err != nil {
			return err
		}
		cloned := clonedValue.(map[string]any)
		if err := validateZipperRaw(cloned); err != nil {
			return err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.DepositWithdrawConfig = nil
		m.Zipper = nil
		m.legacyZipperRaw = cloned
		return nil
	default:
		return &errors.ValidationError{Msg: "unsupported zipper config type"}
	}
}

// HydrateDepositWithdrawConfig stores typed deposit/withdraw config.
func (m *Manager) HydrateDepositWithdrawConfig(config *models.DepositWithdrawConfig) error {
	cloned, err := cloneDepositWithdrawConfig(config)
	if err != nil {
		return err
	}
	if err := validateDepositWithdrawConfig(cloned); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DepositWithdrawConfig = cloned
	m.Zipper = BuildZipperCatalogData(cloned)
	m.legacyZipperRaw = nil
	return nil
}

func cloneDepositWithdrawConfig(config *models.DepositWithdrawConfig) (*models.DepositWithdrawConfig, error) {
	if config == nil {
		return nil, nil
	}
	cloned, err := cloneCatalogValue(config)
	if err != nil {
		return nil, err
	}
	return cloned.(*models.DepositWithdrawConfig), nil
}

type cloneVisit struct {
	kind reflect.Kind
	typ  reflect.Type
	ptr  uintptr
	len  int
	cap  int
}

// cloneCatalogValue recursively copies JSON-shaped pointers, maps, slices,
// arrays, and structs with exported fields while preserving concrete Go types
// and reference topology. Known immutable time.Time values are also allowed.
func cloneCatalogValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	cloned, err := cloneCatalogReflect(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value), "$")
	if err != nil {
		return nil, err
	}
	return cloned.Interface(), nil
}

func cloneCatalogReflect(value reflect.Value, visited map[cloneVisit]reflect.Value, path string) (reflect.Value, error) {
	if !value.IsValid() {
		return value, nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		cloned, err := cloneCatalogReflect(value.Elem(), visited, path)
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out, nil
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		visit := cloneReferenceVisit(value)
		if cloned, ok := visited[visit]; ok {
			return cloned, nil
		}
		out := reflect.New(value.Type().Elem())
		visited[visit] = out
		cloned, err := cloneCatalogReflect(value.Elem(), visited, path)
		if err != nil {
			return reflect.Value{}, err
		}
		out.Elem().Set(cloned)
		return out, nil
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		visit := cloneReferenceVisit(value)
		if cloned, ok := visited[visit]; ok {
			return cloned, nil
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		visited[visit] = out
		iter := value.MapRange()
		for iter.Next() {
			entryPath := cloneMapEntryPath(path, iter.Key())
			key, err := cloneCatalogReflect(iter.Key(), visited, entryPath)
			if err != nil {
				return reflect.Value{}, err
			}
			entry, err := cloneCatalogReflect(iter.Value(), visited, entryPath)
			if err != nil {
				return reflect.Value{}, err
			}
			out.SetMapIndex(key, entry)
		}
		return out, nil
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		visit := cloneReferenceVisit(value)
		if cloned, ok := visited[visit]; ok {
			return cloned, nil
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		visited[visit] = out
		for i := 0; i < value.Len(); i++ {
			cloned, err := cloneCatalogReflect(value.Index(i), visited, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(cloned)
		}
		return out, nil
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned, err := cloneCatalogReflect(value.Index(i), visited, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(cloned)
		}
		return out, nil
	case reflect.Struct:
		if safelyCopyableImmutableStruct(value.Type()) {
			return value, nil
		}
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath != "" {
				return reflect.Value{}, &errors.ValidationError{
					Msg: fmt.Sprintf(
						"catalog value at %s uses struct type %s with unexported fields; raw catalog values must be JSON-shaped",
						path,
						value.Type(),
					),
				}
			}
		}
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			fieldInfo := value.Type().Field(i)
			fieldPath := path + "." + fieldInfo.Name
			cloned, err := cloneCatalogReflect(value.Field(i), visited, fieldPath)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Field(i).Set(cloned)
		}
		return out, nil
	default:
		return value, nil
	}
}

func cloneMapEntryPath(path string, key reflect.Value) string {
	if key.Kind() == reflect.String {
		return path + "." + key.String()
	}
	return path + "[key]"
}

func safelyCopyableImmutableStruct(typ reflect.Type) bool {
	return typ == reflect.TypeFor[time.Time]()
}

func cloneReferenceVisit(value reflect.Value) cloneVisit {
	visit := cloneVisit{
		kind: value.Kind(),
		typ:  value.Type(),
		ptr:  value.Pointer(),
	}
	if value.Kind() == reflect.Slice {
		visit.len = value.Len()
		visit.cap = value.Cap()
	}
	return visit
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

// SymbolForSymbolID resolves display symbol from spot config symbol id.
func (m *Manager) SymbolForSymbolID(symbolID uint32) *string {
	if symbolID == 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		v := intish(pair["symbol_id"])
		if v == nil {
			v = intish(pair["symbolId"])
		}
		if v != nil && *v == symbolID {
			if s, _ := pair["symbol"].(string); s != "" {
				return &s
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
			if v, found := intField(pair, "base_quantity_scale", "baseQuantityScale", "qtyScale"); found {
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
						if scale, found := intField(p, "base_quantity_scale", "baseQuantityScale", "qtyScale"); found {
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

// QuoteQuantityScaleForSymbol returns the pair quote quantity scale.
//
// Quote-debit budgets must use this scale. Callers must not infer it from the
// base quantity scale or from the quote asset's display decimals.
// ok is false when the symbol is unknown, catalogs are unhydrated, or the pair
// omits quote_quantity_scale.
func (m *Manager) QuoteQuantityScaleForSymbol(symbol string) (scale int, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		if s, _ := pair["symbol"].(string); s == symbol {
			if v, found := intField(pair, "quote_quantity_scale", "quoteQuantityScale"); found {
				return v, true
			}
			return 0, false
		}
	}
	return 0, false
}

// QuoteQuantityScaleForSymbolID returns the pair quote quantity scale for symbol id.
//
// ok is false when the id is unknown, catalogs are unhydrated, or the pair
// omits quote_quantity_scale.
func (m *Manager) QuoteQuantityScaleForSymbolID(symbolID uint32) (scale int, ok bool) {
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
						if scale, found := intField(p, "quote_quantity_scale", "quoteQuantityScale"); found {
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

// PairConstraintsForSymbol returns exact, parsed trading constraints for symbol.
// ok is false when the symbol is unknown or the catalog omits/malforms a rule.
// Zero-valued optional minima (`min_qty_base` / `min_notional_quote` of "0")
// are treated as unset so live catalogs remain usable for inspection.
func (m *Manager) PairConstraintsForSymbol(symbol string) (models.PairConstraints, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return pairConstraints(m.pairForSymbolLocked(symbol))
}

// PairConstraintsForSymbolID returns exact, parsed trading constraints for a
// symbol id. It never invents defaults for missing catalog fields.
func (m *Manager) PairConstraintsForSymbolID(symbolID uint32) (models.PairConstraints, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pair := range m.pairsLocked() {
		id := intish(pair["symbol_id"])
		if id == nil {
			id = intish(pair["symbolId"])
		}
		if id != nil && *id == symbolID {
			return pairConstraints(pair)
		}
	}
	return models.PairConstraints{}, false
}

func pairConstraints(pair map[string]any) (models.PairConstraints, bool) {
	if pair == nil {
		return models.PairConstraints{}, false
	}
	symbol, _ := pair["symbol"].(string)
	id := intish(pair["symbol_id"])
	if id == nil {
		id = intish(pair["symbolId"])
	}
	baseScale, baseOK := intField(pair, "base_quantity_scale", "baseQuantityScale", "qtyScale")
	quoteScale, quoteOK := intField(pair, "quote_quantity_scale", "quoteQuantityScale")
	tick := stringField(pair, "tick_size", "tickSize")
	step := stringField(pair, "step_size", "stepSize")
	minQty := stringField(pair, "min_qty_base", "minQtyBase")
	minNotional := stringField(pair, "min_notional_quote", "minNotionalQuote")
	if symbol == "" || id == nil || !baseOK {
		return models.PairConstraints{}, false
	}
	tickTicks, ok := optionalScaledDecimal(tick, 6, false)
	if !ok {
		return models.PairConstraints{}, false
	}
	stepScaled, ok := optionalScaledDecimal(step, baseScale, false)
	if !ok {
		return models.PairConstraints{}, false
	}
	minQtyScaled, ok := optionalScaledDecimal(minQty, baseScale, true)
	if !ok {
		return models.PairConstraints{}, false
	}
	minNotionalComputable := false
	if minNotional != "" {
		value, valid := new(big.Rat).SetString(minNotional)
		if !valid || value.Sign() < 0 {
			return models.PairConstraints{}, false
		}
		if value.Sign() == 0 {
			minNotional = ""
		} else if quoteOK {
			minNotionalComputable = true
		}
	}
	return models.PairConstraints{
		Symbol:                symbol,
		SymbolID:              *id,
		BaseQuantityScale:     baseScale,
		QuoteQuantityScale:    quoteScale,
		TickSize:              tick,
		TickSizeTicks:         tickTicks,
		StepSize:              step,
		StepSizeScaled:        stepScaled,
		MinQtyBase:            minQty,
		MinQtyScaled:          minQtyScaled,
		MinNotionalQuote:      minNotional,
		MinNotionalComputable: minNotionalComputable,
	}, true
}

func optionalScaledDecimal(raw string, scale int, zeroIsUnset bool) (int64, bool) {
	if raw == "" {
		return 0, true
	}
	value, ok := scaledDecimalInt64(raw, scale)
	if !ok || value < 0 || (value == 0 && !zeroIsUnset) {
		return 0, false
	}
	return value, true
}

func stringField(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key].(string); ok {
			return value
		}
	}
	return ""
}

func scaledDecimalInt64(raw string, scale int) (int64, bool) {
	if scale < 0 || scale > maxProtocolScale {
		return 0, false
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok || value.Sign() < 0 {
		return 0, false
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(factor))
	if !scaled.IsInt() || !scaled.Num().IsInt64() {
		return 0, false
	}
	return scaled.Num().Int64(), true
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
			if v, found := intField(item, "quantityScale", "quantity_scale"); found {
				return &v
			}
		}
	}
	return nil
}

// QuantityScaleForZippedAssetID returns scale for zipped asset id.
func (m *Manager) QuantityScaleForZippedAssetID(zippedAssetID uint32) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Zipper != nil {
		for _, asset := range m.Zipper.Assets {
			for _, chain := range asset.Chains {
				if chain.ZippedAssetID == zippedAssetID {
					return asset.QuantityScale, true
				}
			}
		}
	}
	return 0, false
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
		return &errors.ValidationError{Msg: "spot catalog is required"}
	}
	symbols := map[string]struct{}{}
	ids := map[uint32]struct{}{}
	count := 0
	for _, key := range []string{"pairs", "symbols"} {
		raw, _ := config[key].([]any)
		for i, item := range raw {
			pair, ok := item.(map[string]any)
			if !ok {
				return &errors.ValidationError{Msg: fmt.Sprintf("spot %s[%d] must be an object", key, i)}
			}
			symbol, _ := pair["symbol"].(string)
			if symbol == "" {
				return &errors.ValidationError{Msg: fmt.Sprintf("spot %s[%d] symbol is required", key, i)}
			}
			id, found, err := requiredU32Field(pair, "symbol_id", "symbolId")
			if err != nil || !found || id == 0 {
				if err == nil {
					err = &errors.ValidationError{Msg: "symbol_id must be non-zero"}
				}
				return fmt.Errorf("spot %s[%d]: %w", key, i, err)
			}
			if err := validateRequiredScaleField(pair, "base_quantity_scale", "baseQuantityScale", "qtyScale"); err != nil {
				return fmt.Errorf("spot %s[%d]: %w", key, i, err)
			}
			if err := validateOptionalScaleField(pair, "quote_quantity_scale", "quoteQuantityScale"); err != nil {
				return fmt.Errorf("spot %s[%d]: %w", key, i, err)
			}
			if _, exists := symbols[symbol]; exists {
				return &errors.ValidationError{Msg: fmt.Sprintf("spot catalog contains duplicate symbol %q", symbol)}
			}
			if _, exists := ids[id]; exists {
				return &errors.ValidationError{Msg: fmt.Sprintf("spot catalog contains duplicate symbol_id %d", id)}
			}
			symbols[symbol] = struct{}{}
			ids[id] = struct{}{}
			count++
		}
	}
	if count == 0 {
		return &errors.ValidationError{Msg: "spot catalog empty or missing pairs"}
	}
	return nil
}

func validateZipperRaw(config map[string]any) error {
	if config == nil {
		return &errors.ValidationError{Msg: "zipper catalog is required"}
	}
	assetNames := map[string]struct{}{}
	ledgerIDs := map[uint32]struct{}{}
	zippedIDs := map[uint32]struct{}{}
	assets, _ := config["assets"].([]any)
	for i, row := range assets {
		item, ok := row.(map[string]any)
		if !ok {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper assets[%d] must be an object", i)}
		}
		asset, _ := item["asset"].(string)
		if asset == "" {
			asset, _ = item["code"].(string)
		}
		if asset == "" {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper assets[%d] asset is required", i)}
		}
		ledgerID, found, err := requiredU32Field(item, "ledger_id", "ledgerId")
		if err != nil || !found || ledgerID == 0 {
			if err == nil {
				err = &errors.ValidationError{Msg: "ledger_id must be non-zero"}
			}
			return fmt.Errorf("zipper assets[%d]: %w", i, err)
		}
		if err := validateRequiredScaleField(item, "quantity_scale", "quantityScale"); err != nil {
			return fmt.Errorf("zipper assets[%d]: %w", i, err)
		}
		if _, exists := assetNames[asset]; exists {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate asset %q", asset)}
		}
		if _, exists := ledgerIDs[ledgerID]; exists {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate ledger_id %d", ledgerID)}
		}
		assetNames[asset] = struct{}{}
		ledgerIDs[ledgerID] = struct{}{}
		variants, _ := item["variants"].([]any)
		for j, rawVariant := range variants {
			variant, ok := rawVariant.(map[string]any)
			if !ok {
				return &errors.ValidationError{Msg: fmt.Sprintf("zipper assets[%d] variants[%d] must be an object", i, j)}
			}
			zippedID, found, err := requiredU32Field(variant, "zipped_asset_id", "zippedAssetId")
			if err != nil || !found || zippedID == 0 {
				if err == nil {
					err = &errors.ValidationError{Msg: "zipped_asset_id must be non-zero"}
				}
				return fmt.Errorf("zipper assets[%d] variants[%d]: %w", i, j, err)
			}
			if _, exists := zippedIDs[zippedID]; exists {
				return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate zipped_asset_id %d", zippedID)}
			}
			zippedIDs[zippedID] = struct{}{}
		}
	}
	return nil
}

func validateDepositWithdrawConfig(config *models.DepositWithdrawConfig) error {
	if config == nil {
		return nil
	}
	assetNames := map[string]struct{}{}
	ledgerIDs := map[uint32]struct{}{}
	zippedIDs := map[uint32]struct{}{}
	for i, asset := range config.Assets {
		if asset.Asset == "" || asset.LedgerID == 0 {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper assets[%d] requires asset and non-zero ledger_id", i)}
		}
		if asset.QuantityScale < 0 || asset.QuantityScale > maxProtocolScale {
			return &errors.ValidationError{
				Msg: fmt.Sprintf("zipper assets[%d] quantity_scale %d exceeds maximum protocol scale %d", i, asset.QuantityScale, maxProtocolScale),
			}
		}
		if _, ok := assetNames[asset.Asset]; ok {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate asset %q", asset.Asset)}
		}
		if _, ok := ledgerIDs[asset.LedgerID]; ok {
			return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate ledger_id %d", asset.LedgerID)}
		}
		assetNames[asset.Asset] = struct{}{}
		ledgerIDs[asset.LedgerID] = struct{}{}
		for j, variant := range asset.Variants {
			if variant.ZippedAssetID == 0 {
				return &errors.ValidationError{Msg: fmt.Sprintf("zipper assets[%d] variants[%d] requires non-zero zipped_asset_id", i, j)}
			}
			if _, ok := zippedIDs[variant.ZippedAssetID]; ok {
				return &errors.ValidationError{Msg: fmt.Sprintf("zipper catalog contains duplicate zipped_asset_id %d", variant.ZippedAssetID)}
			}
			zippedIDs[variant.ZippedAssetID] = struct{}{}
		}
	}
	return nil
}

func requiredU32Field(row map[string]any, keys ...string) (uint32, bool, error) {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			parsed, err := parseU32Exact(value, key)
			return parsed, true, err
		}
	}
	return 0, false, nil
}

func validateRequiredScaleField(row map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, ok := row[key]; ok {
			return validateOptionalScaleField(row, keys...)
		}
	}
	return &errors.ValidationError{Msg: "quantity scale is required"}
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

func intField(row map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			return intValue(value), true
		}
	}
	return 0, false
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "0" {
		return "0"
	}
	return s
}
