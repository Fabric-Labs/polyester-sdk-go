package models

import "time"

// ZipperTokenConfig describes an on-chain token.
type ZipperTokenConfig struct {
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

// ZipperAssetChainVariant is per-chain asset variant config.
type ZipperAssetChainVariant struct {
	ZippedAssetID     uint32 `json:"zipped_asset_id"`
	ChainID           uint32 `json:"chain_id"`
	IsNativeAsset     bool   `json:"is_native_asset,omitempty"`
	NetworkFee        string `json:"network_fee,omitempty"`
	NetworkFeeTsSec   int64  `json:"network_fee_ts_sec,omitempty"`
	DepositMinAmount  string `json:"deposit_min_amount,omitempty"`
	WithdrawMinAmount string `json:"withdraw_min_amount,omitempty"`
	// Supply is the route's zToken supply, a decimal in the asset's units,
	// set only while SupplyStatus is SupplyStatusAvailable or
	// SupplyStatusStale ("0" is then a real supply of zero); empty when no
	// supply is known. Never read an empty Supply as zero.
	Supply string `json:"supply,omitempty"`
	// SupplyStatus says whether Supply is known and current:
	// SupplyStatusAvailable, SupplyStatusStale (not refreshed for longer than
	// expected; it may no longer be accurate), SupplyStatusMissing (no supply
	// is known yet), "" when the server names none (read it as missing) and
	// "unknown(n)" for a status this SDK does not know. Size a withdrawal
	// against Supply only while it is SupplyStatusAvailable.
	SupplyStatus string `json:"supply_status,omitempty"`
	// SupplyObservedAt is when Supply was read from Polyester chain; nil
	// when no supply is known.
	SupplyObservedAt *time.Time        `json:"supply_observed_at,omitempty"`
	SourceToken      ZipperTokenConfig `json:"source_token,omitempty"`
	ZToken           ZipperTokenConfig `json:"z_token,omitempty"`
	// DepositStatus says whether the route accepts new deposits:
	// RouteStatusNormal or RouteStatusSuspended; "" while availability is not
	// known, which does not mean available, and "unknown(n)" for a status this
	// SDK version does not know. Use AcceptsDeposits.
	DepositStatus string `json:"deposit_status,omitempty"`
	// WithdrawalStatus says whether the route accepts new withdrawals, with
	// the same labels as DepositStatus. Use AcceptsWithdrawals.
	WithdrawalStatus string `json:"withdrawal_status,omitempty"`
}

// Supply statuses of ZipperAssetChainVariant.SupplyStatus and
// ZippedAssetSupplyUpdate.Status.
const (
	// SupplyStatusAvailable: Supply was read from Polyester chain recently
	// and is current.
	SupplyStatusAvailable = "available"
	// SupplyStatusStale: Supply is the last value read, but it has not been
	// refreshed for longer than expected and may no longer be accurate. New
	// withdrawals to the route are refused with
	// ERROR_CODE_SUPPLY_UNAVAILABLE until it is available again.
	SupplyStatusStale = "stale"
	// SupplyStatusMissing: no supply is known for the route yet. New
	// withdrawals to it are refused with ERROR_CODE_SUPPLY_UNAVAILABLE.
	SupplyStatusMissing = "missing"
)

// SupplyAvailable reports whether the route's Supply is known and current
// (SupplyStatusAvailable): the only status a withdrawal may be sized
// against. Treat any other status as "liquidity cannot be confirmed right
// now", not as zero.
func (v ZipperAssetChainVariant) SupplyAvailable() bool {
	return v.SupplyStatus == SupplyStatusAvailable
}

// SupplyAvailable reports whether the route's Supply is known and current;
// see ZipperAssetChainVariant.SupplyAvailable.
func (c ZipperEnrichedAssetChain) SupplyAvailable() bool {
	return c.SupplyStatus == SupplyStatusAvailable
}

// Route statuses of ZipperAssetChainVariant.DepositStatus and
// WithdrawalStatus. A suspension can stop one direction and not the other,
// and no reason is published.
const (
	// RouteStatusNormal: the route accepts new requests.
	RouteStatusNormal = "normal"
	// RouteStatusSuspended: the route does not accept new requests. Requests
	// created before the suspension continue to process, and funds sent to a
	// deposit address while deposits are suspended are delayed, not lost.
	RouteStatusSuspended = "suspended"
)

// AcceptsDeposits reports whether the route currently accepts new deposits:
// true only for RouteStatusNormal. An unknown status is not available.
func (v ZipperAssetChainVariant) AcceptsDeposits() bool {
	return v.DepositStatus == RouteStatusNormal
}

// AcceptsWithdrawals reports whether the route currently accepts new
// withdrawals: true only for RouteStatusNormal. An unknown status is not
// available.
func (v ZipperAssetChainVariant) AcceptsWithdrawals() bool {
	return v.WithdrawalStatus == RouteStatusNormal
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
	// MemoRequirement is the network's rule for a memo or destination tag on
	// a withdrawal destination: MemoRequirementRequired, MemoRequirementOptional
	// or MemoRequirementNotSupported. Empty when the server names no rule, which
	// does not mean a memo can be omitted, and "UNKNOWN(n)" for a rule this SDK
	// version does not know. It does not describe deposit instructions.
	MemoRequirement string `json:"memo_requirement,omitempty"`
	// MemoLabel is the display name of the memo, such as "Destination tag" or
	// "Memo". Empty when the network does not support one.
	MemoLabel string `json:"memo_label,omitempty"`
	// MemoFormatHint is human-readable format guidance, as sent, for display
	// only: validate against MemoRequirement, not this text.
	MemoFormatHint string `json:"memo_format_hint,omitempty"`
}

// Memo rules of ZipperChainConfig.MemoRequirement.
const (
	// MemoRequirementRequired: a memo or destination tag must be supplied.
	MemoRequirementRequired = "required"
	// MemoRequirementOptional: a memo or destination tag may be supplied.
	MemoRequirementOptional = "optional"
	// MemoRequirementNotSupported: the memo or destination tag must be empty.
	MemoRequirementNotSupported = "not_supported"
)

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
	ZippedAssetID     uint32 `json:"zipped_asset_id,omitempty"`
	IsNativeAsset     bool   `json:"is_native_asset,omitempty"`
	NetworkFee        string `json:"network_fee,omitempty"`
	NetworkFeeTsSec   int64  `json:"network_fee_ts_sec,omitempty"`
	DepositMinAmount  string `json:"deposit_min_amount,omitempty"`
	WithdrawMinAmount string `json:"withdraw_min_amount,omitempty"`
	// Supply is the route's zToken supply, a decimal in the asset's units,
	// set only while SupplyStatus is SupplyStatusAvailable or
	// SupplyStatusStale ("0" is then a real supply of zero); empty when no
	// supply is known. Never read an empty Supply as zero.
	Supply string `json:"supply,omitempty"`
	// SupplyStatus says whether Supply is known and current:
	// SupplyStatusAvailable, SupplyStatusStale (not refreshed for longer than
	// expected; it may no longer be accurate), SupplyStatusMissing (no supply
	// is known yet), "" when the server names none (read it as missing) and
	// "unknown(n)" for a status this SDK does not know. Size a withdrawal
	// against Supply only while it is SupplyStatusAvailable.
	SupplyStatus string `json:"supply_status,omitempty"`
	// SupplyObservedAt is when Supply was read from Polyester chain; nil
	// when no supply is known.
	SupplyObservedAt *time.Time        `json:"supply_observed_at,omitempty"`
	SourceToken      ZipperTokenConfig `json:"source_token,omitempty"`
	ZToken           ZipperTokenConfig `json:"z_token,omitempty"`
	// DepositStatus and WithdrawalStatus are the route's
	// ZipperAssetChainVariant statuses.
	DepositStatus    string `json:"deposit_status,omitempty"`
	WithdrawalStatus string `json:"withdrawal_status,omitempty"`
}

// AcceptsDeposits reports whether the route currently accepts new deposits:
// true only for RouteStatusNormal.
func (c ZipperEnrichedAssetChain) AcceptsDeposits() bool {
	return c.DepositStatus == RouteStatusNormal
}

// AcceptsWithdrawals reports whether the route currently accepts new
// withdrawals: true only for RouteStatusNormal.
func (c ZipperEnrichedAssetChain) AcceptsWithdrawals() bool {
	return c.WithdrawalStatus == RouteStatusNormal
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
	Chains    []ZipperChainConfig         `json:"chains,omitempty"`
	Assets    []ZipperAssetConfig         `json:"assets,omitempty"`
	Contracts []ZipperChainContractConfig `json:"contracts,omitempty"`
	TsMs      int64                       `json:"ts_ms,omitempty"`
}

// ZippedAssetSupplyUpdate is a supply patch row.
type ZippedAssetSupplyUpdate struct {
	ZippedAssetID uint32 `json:"zipped_asset_id"`
	// Supply is the route's supply, a decimal in the asset's units; empty
	// when the update names no known supply.
	Supply string `json:"supply,omitempty"`
	// Status is SupplyStatusAvailable or SupplyStatusStale: a route whose
	// supply stops being refreshed gets one stale update repeating its last
	// value and observation time, and its next reading makes it available
	// again. "" when the server names none, "unknown(n)" for a status this
	// SDK does not know.
	Status string `json:"status,omitempty"`
	// ObservedAt is when Supply was read from Polyester chain; nil when the
	// update names none.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

// ZippedAssetSupplyBatch is a realtime supply batch.
type ZippedAssetSupplyBatch struct {
	Updates []ZippedAssetSupplyUpdate `json:"updates,omitempty"`
}
