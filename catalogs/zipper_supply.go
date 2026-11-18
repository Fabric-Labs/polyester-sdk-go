package catalogs

import (
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func sameTime(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

// PatchZipperCatalogSupply returns a catalog copy with supply updates
// applied: each route's Supply, SupplyStatus and SupplyObservedAt.
func PatchZipperCatalogSupply(catalog *models.ZipperCatalogData, updates []models.ZippedAssetSupplyUpdate) *models.ZipperCatalogData {
	if catalog == nil || len(updates) == 0 {
		return catalog
	}
	byID := map[uint32]models.ZippedAssetSupplyUpdate{}
	for _, u := range updates {
		byID[u.ZippedAssetID] = u
	}
	changed := false
	assets := make([]models.ZipperEnrichedAssetConfig, len(catalog.Assets))
	copy(assets, catalog.Assets)
	for i, asset := range assets {
		chains := make([]models.ZipperEnrichedAssetChain, len(asset.Chains))
		copy(chains, asset.Chains)
		for j, chain := range chains {
			update, ok := byID[chain.ZippedAssetID]
			if !ok {
				continue
			}
			patched := chain
			patched.Supply = update.Supply
			patched.SupplyStatus = update.Status
			patched.SupplyObservedAt = update.ObservedAt
			if patched.Supply != chain.Supply || patched.SupplyStatus != chain.SupplyStatus || !sameTime(patched.SupplyObservedAt, chain.SupplyObservedAt) {
				chains[j] = patched
				changed = true
			}
		}
		assets[i].Chains = chains
	}
	if !changed {
		return catalog
	}
	out := *catalog
	out.Assets = assets
	return &out
}
