package catalogs

import "github.com/Fabric-Labs/polyester-sdk-go/models"

// BuildZipperCatalogData enriches deposit/withdraw config for catalog lookups.
func BuildZipperCatalogData(config *models.DepositWithdrawConfig) *models.ZipperCatalogData {
	if config == nil {
		return nil
	}
	chainsByID := map[uint32]models.ZipperChainConfig{}
	for _, chain := range config.Chains {
		chainsByID[chain.ChainID] = chain
	}
	enriched := make([]models.ZipperEnrichedAssetConfig, 0, len(config.Assets))
	for _, asset := range config.Assets {
		enriched = append(enriched, models.ZipperEnrichedAssetConfig{
			Asset:                   asset.Asset,
			LedgerID:                asset.LedgerID,
			Name:                    asset.Name,
			Icon:                    asset.Icon,
			QuantityScale:           asset.QuantityScale,
			QuantityDisplayDecimals: asset.QuantityDisplayDecimals,
			UAssetID:                asset.UAssetID,
			Chains:                  enrichAssetChains(asset, chainsByID),
		})
	}
	return &models.ZipperCatalogData{
		Chains:    config.Chains,
		Assets:    enriched,
		Contracts: config.Contracts,
		TsMs:      config.TsMs,
	}
}

func enrichAssetChains(asset models.ZipperAssetConfig, chainsByID map[uint32]models.ZipperChainConfig) []models.ZipperEnrichedAssetChain {
	out := make([]models.ZipperEnrichedAssetChain, 0, len(asset.Variants))
	for _, variant := range asset.Variants {
		chain, ok := chainsByID[variant.ChainID]
		if !ok {
			continue
		}
		out = append(out, models.ZipperEnrichedAssetChain{
			ZipperChainConfig: chain,
			ZippedAssetID:     variant.ZippedAssetID,
			IsNativeAsset:     variant.IsNativeAsset,
			NetworkFee:        variant.NetworkFee,
			NetworkFeeTsSec:   variant.NetworkFeeTsSec,
			DepositMinAmount:  variant.DepositMinAmount,
			WithdrawMinAmount: variant.WithdrawMinAmount,
			Supply:            variant.Supply,
			SourceToken:       variant.SourceToken,
			ZToken:            variant.ZToken,
		})
	}
	return out
}
