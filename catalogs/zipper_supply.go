package catalogs

import "github.com/Fabric-Labs/polyester-sdk-go/models"

// PatchZipperCatalogSupply returns a catalog copy with supply updates applied.
func PatchZipperCatalogSupply(catalog *models.ZipperCatalogData, updates []models.ZippedAssetSupplyUpdate) *models.ZipperCatalogData {
	if catalog == nil || len(updates) == 0 {
		return catalog
	}
	byID := map[uint32]string{}
	for _, u := range updates {
		byID[u.ZippedAssetID] = u.Supply
	}
	changed := false
	assets := make([]models.ZipperEnrichedAssetConfig, len(catalog.Assets))
	copy(assets, catalog.Assets)
	for i, asset := range assets {
		chains := make([]models.ZipperEnrichedAssetChain, len(asset.Chains))
		copy(chains, asset.Chains)
		for j, chain := range chains {
			if supply, ok := byID[chain.ZippedAssetID]; ok && supply != chain.Supply {
				chains[j].Supply = supply
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
