package models

// ZipperTokenConfig describes an on-chain token.
type ZipperTokenConfig struct {
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

// ZipperAssetChainVariant is per-chain asset variant config.
type ZipperAssetChainVariant struct {
	ZippedAssetID     uint32            `json:"zipped_asset_id"`
	ChainID           uint32            `json:"chain_id"`
	IsNativeAsset     bool              `json:"is_native_asset,omitempty"`
	NetworkFee        string            `json:"network_fee,omitempty"`
	NetworkFeeTsSec   int64             `json:"network_fee_ts_sec,omitempty"`
	DepositMinAmount  string            `json:"deposit_min_amount,omitempty"`
	WithdrawMinAmount string            `json:"withdraw_min_amount,omitempty"`
	Supply            string            `json:"supply,omitempty"`
	SourceToken       ZipperTokenConfig `json:"source_token,omitempty"`
	ZToken            ZipperTokenConfig `json:"z_token,omitempty"`
}

// ZipperChainConfig is chain metadata.
type ZipperChainConfig struct {
	ChainID                 uint32 `json:"chain_id"`
	Code                    string `json:"code,omitempty"`
	Name                    string `json:"name,omitempty"`
	NativeChainID           string `json:"native_chain_id,omitempty"`
	NativeCurrencySymbol    string `json:"native_currency_symbol,omitempty"`
	ExplorerURL             string `json:"explorer_url,omitempty"`
	Icon                    string `json:"icon,omitempty"`
	RequiredConfirmations   int    `json:"required_confirmations,omitempty"`
	ConfirmationTimeSeconds int    `json:"confirmation_time_seconds,omitempty"`
	IsCaseSensitive         bool   `json:"is_case_sensitive,omitempty"`
	MinAddressLength        int    `json:"min_address_length,omitempty"`
	MaxAddressLength        int    `json:"max_address_length,omitempty"`
}

// ZipperAssetConfig is asset metadata.
type ZipperAssetConfig struct {
	Asset                   string                    `json:"asset,omitempty"`
	LedgerID                uint32                    `json:"ledger_id,omitempty"`
	Name                    string                    `json:"name,omitempty"`
	Icon                    string                    `json:"icon,omitempty"`
	QuantityScale           int                       `json:"quantity_scale,omitempty"`
	QuantityDisplayDecimals int                       `json:"quantity_display_decimals,omitempty"`
	UAssetID                string                    `json:"u_asset_id,omitempty"`
	Variants                []ZipperAssetChainVariant `json:"variants,omitempty"`
}

// ZipperChainContractConfig is a chain contract entry.
type ZipperChainContractConfig struct {
	Name        string `json:"name,omitempty"`
	Address     string `json:"address,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Version     int    `json:"version,omitempty"`
}

// ZipperEnrichedAssetChain joins asset variant with chain metadata.
type ZipperEnrichedAssetChain struct {
	ZipperChainConfig
	ZippedAssetID     uint32            `json:"zipped_asset_id,omitempty"`
	IsNativeAsset     bool              `json:"is_native_asset,omitempty"`
	NetworkFee        string            `json:"network_fee,omitempty"`
	NetworkFeeTsSec   int64             `json:"network_fee_ts_sec,omitempty"`
	DepositMinAmount  string            `json:"deposit_min_amount,omitempty"`
	WithdrawMinAmount string            `json:"withdraw_min_amount,omitempty"`
	Supply            string            `json:"supply,omitempty"`
	SourceToken       ZipperTokenConfig `json:"source_token,omitempty"`
	ZToken            ZipperTokenConfig `json:"z_token,omitempty"`
}

// ZipperEnrichedAssetConfig is catalog-enriched asset config.
type ZipperEnrichedAssetConfig struct {
	Asset                   string                     `json:"asset,omitempty"`
	LedgerID                uint32                     `json:"ledger_id,omitempty"`
	Name                    string                     `json:"name,omitempty"`
	Icon                    string                     `json:"icon,omitempty"`
	QuantityScale           int                        `json:"quantity_scale,omitempty"`
	QuantityDisplayDecimals int                        `json:"quantity_display_decimals,omitempty"`
	UAssetID                string                     `json:"u_asset_id,omitempty"`
	Chains                  []ZipperEnrichedAssetChain `json:"chains,omitempty"`
}

// ZipperCatalogData is the typed zipper catalog.
type ZipperCatalogData struct {
	Chains    []ZipperChainConfig         `json:"chains,omitempty"`
	Assets    []ZipperEnrichedAssetConfig `json:"assets,omitempty"`
	Contracts []ZipperChainContractConfig `json:"contracts,omitempty"`
	TsMs      int64                       `json:"ts_ms,omitempty"`
}

// DepositWithdrawConfig is the zipper deposit/withdraw config payload.
type DepositWithdrawConfig struct {
	Chains           []ZipperChainConfig         `json:"chains,omitempty"`
	Assets           []ZipperAssetConfig         `json:"assets,omitempty"`
	Contracts        []ZipperChainContractConfig `json:"contracts,omitempty"`
	PolyesterChainID uint32                      `json:"polyester_chain_id,omitempty"`
	TsMs             int64                       `json:"ts_ms,omitempty"`
}

// ZippedAssetSupplyUpdate is a supply patch row.
type ZippedAssetSupplyUpdate struct {
	ZippedAssetID uint32 `json:"zipped_asset_id"`
	Supply        string `json:"supply,omitempty"`
}

// ZippedAssetSupplyBatch is a realtime supply batch.
type ZippedAssetSupplyBatch struct {
	Updates []ZippedAssetSupplyUpdate `json:"updates,omitempty"`
}
