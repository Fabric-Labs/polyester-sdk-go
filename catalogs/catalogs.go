package catalogs

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Manager holds hydrated spot and zipper catalogs.
type Manager struct {
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

// HydrateSpotConfig stores spot pair config.
func (m *Manager) HydrateSpotConfig(config map[string]any) {
	m.SpotConfig = config
}

// HydrateZipperConfig stores zipper catalog config.
func (m *Manager) HydrateZipperConfig(config any) {
	switch c := config.(type) {
	case models.DepositWithdrawConfig:
		m.HydrateDepositWithdrawConfig(&c)
	case *models.DepositWithdrawConfig:
		m.HydrateDepositWithdrawConfig(c)
	case map[string]any:
		m.DepositWithdrawConfig = nil
		m.Zipper = nil
		m.legacyZipperRaw = c
	}
}

// HydrateDepositWithdrawConfig stores typed deposit/withdraw config.
func (m *Manager) HydrateDepositWithdrawConfig(config *models.DepositWithdrawConfig) {
	m.DepositWithdrawConfig = config
	m.Zipper = BuildZipperCatalogData(config)
}

// SymbolIDForSymbol resolves symbol id from spot config.
func (m *Manager) SymbolIDForSymbol(symbol string) *uint32 {
	for _, pair := range m.pairs() {
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
func (m *Manager) BaseQuantityScaleForSymbol(symbol string) int {
	for _, pair := range m.pairs() {
		if s, _ := pair["symbol"].(string); s == symbol {
			if v := intValue(pair["base_quantity_scale"]); v > 0 {
				return v
			}
			if v := intValue(pair["baseQuantityScale"]); v > 0 {
				return v
			}
			if v := intValue(pair["qtyScale"]); v > 0 {
				return v
			}
		}
	}
	return 8
}

// BaseQuantityScaleForSymbolID returns qty scale for symbol id.
func (m *Manager) BaseQuantityScaleForSymbolID(symbolID uint32) int {
	for _, pair := range m.pairs() {
		v := intish(pair["symbol_id"])
		if v == nil {
			v = intish(pair["symbolId"])
		}
		if v != nil && *v == symbolID {
			if sym, _ := pair["symbol"].(string); sym != "" {
				return m.BaseQuantityScaleForSymbol(sym)
			}
			break
		}
	}
	return 8
}

// OrderbookPriceBucketsForSymbol returns configured price buckets.
func (m *Manager) OrderbookPriceBucketsForSymbol(symbol string) []string {
	pair := m.pairForSymbol(symbol)
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
		raw = m.ZipperConfig()
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
		raw = m.ZipperConfig()
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

func (m *Manager) pairForSymbol(symbol string) map[string]any {
	for _, pair := range m.pairs() {
		if s, _ := pair["symbol"].(string); s == symbol {
			return pair
		}
	}
	return nil
}

func (m *Manager) pairs() []map[string]any {
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

func intish(v any) *uint32 {
	switch t := v.(type) {
	case float64:
		u := uint32(t)
		return &u
	case int:
		u := uint32(t)
		return &u
	case int64:
		u := uint32(t)
		return &u
	case string:
		n, err := strconv.ParseUint(t, 10, 32)
		if err != nil {
			return nil
		}
		u := uint32(n)
		return &u
	default:
		return nil
	}
}

func intValue(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "0" {
		return "0"
	}
	return s
}
