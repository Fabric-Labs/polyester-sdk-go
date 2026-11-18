package decode

import (
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// DepositWithdrawConfigFromProto decodes the deposit and withdraw config. A
// route whose supply is known (AVAILABLE or STALE) without the time it was
// read returns a *errors.ResponseContractError.
func DepositWithdrawConfigFromProto(msg *zipperv1.GetDepositWithdrawConfigResponse) (models.DepositWithdrawConfig, error) {
	cfg := models.DepositWithdrawConfig{
		TsMs: int64(msg.GetTsSec()) * 1000,
	}
	for _, c := range msg.GetChains() {
		cfg.Chains = append(cfg.Chains, models.ZipperChainConfig{
			ChainID: c.GetChainId(), Code: c.GetCode(), Name: c.GetName(), NativeChainID: c.GetNativeChainId(),
			NativeCurrencySymbol: c.GetNativeCurrencySymbol(), ExplorerURL: c.GetExplorerUrl(), Icon: c.GetIcon(),
			RequiredConfirmations: int(c.GetRequiredConfirmations()), ConfirmationTimeSeconds: int(c.GetConfirmationTimeSeconds()),
			IsCaseSensitive: c.GetIsCaseSensitive(), MinAddressLength: int(c.GetMinAddressLength()), MaxAddressLength: int(c.GetMaxAddressLength()),
			MemoRequirement: memoRequirementLabel(c.GetMemoRequirement()), MemoLabel: c.GetMemoLabel(), MemoFormatHint: c.GetMemoFormatHint(),
		})
	}
	for _, a := range msg.GetAssets() {
		asset := models.ZipperAssetConfig{
			Asset: a.GetAsset(), LedgerID: a.GetLedgerId(), Name: a.GetName(), Icon: a.GetIcon(),
			QuantityScale: int(a.GetQuantityScale()), QuantityDisplayDecimals: int(a.GetQuantityDisplayDecimals()), UAssetID: a.GetUAssetId(),
		}
		for _, v := range a.GetVariants() {
			variant := models.ZipperAssetChainVariant{
				ZippedAssetID: v.GetZippedAssetId(), ChainID: v.GetChainId(), IsNativeAsset: v.GetIsNativeAsset(),
				NetworkFee: v.GetNetworkFee(), DepositMinAmount: v.GetDepositMinAmount(), WithdrawMinAmount: v.GetWithdrawMinAmount(),
				SupplyStatus: supplyStatusLabel(v.GetSupplyStatus()),
				SourceToken:  models.ZipperTokenConfig{Address: v.GetSourceAddress(), Decimals: int(v.GetSourceDecimals())},
				ZToken:       models.ZipperTokenConfig{Address: v.GetZtokenAddress(), Decimals: int(v.GetZtokenDecimals())},
				// STATUS_UNSPECIFIED reads "": not known, so not available.
				DepositStatus:    routeStatusLabel(v.GetDepositStatus()),
				WithdrawalStatus: routeStatusLabel(v.GetWithdrawalStatus()),
			}
			// supply_q is meaningful only while the supply is known
			// (AVAILABLE or STALE), and is scaled by the parent asset's
			// quantity_scale. Otherwise no supply is known: never zero.
			if supplyKnown(v.GetSupplyStatus()) {
				if v.GetSupplyObservedAt() == nil {
					return models.DepositWithdrawConfig{}, &errors.ResponseContractError{
						Operation: "GetDepositWithdrawConfig",
						Msg:       fmt.Sprintf("zipped asset %d has a known supply without supply_observed_at", v.GetZippedAssetId()),
					}
				}
				variant.Supply = formatLedgerU128OrZero(strconv.FormatUint(v.GetSupplyQ(), 10), int(a.GetQuantityScale()))
				variant.SupplyObservedAt = timestampTime(v.GetSupplyObservedAt())
			}
			asset.Variants = append(asset.Variants, variant)
		}
		cfg.Assets = append(cfg.Assets, asset)
	}
	for _, c := range msg.GetContracts() {
		cfg.Contracts = append(cfg.Contracts, models.ZipperChainContractConfig{
			Name: c.GetName(), Address: c.GetAddress(), Type: c.GetType(),
			Description: c.GetDescription(), Version: int(c.GetVersion()),
		})
	}
	return cfg, nil
}

// supplyKnown reports whether a route's supply_q is a known supply: AVAILABLE
// or STALE.
func supplyKnown(status zipperv1.SupplyStatus) bool {
	return status == zipperv1.SupplyStatus_AVAILABLE || status == zipperv1.SupplyStatus_STALE
}

// supplyStatusLabel names a route's supply status: "available", "stale",
// "missing", "" when unspecified and "unknown(n)" for a newer one.
func supplyStatusLabel(status zipperv1.SupplyStatus) string {
	return enumLabel(status, "")
}

func routeStatusLabel(status zipperv1.RouteStatus) string {
	return codecs.ProtoEnumName(status, status.Number())
}

func memoRequirementLabel(requirement zipperv1.MemoRequirement) string {
	switch requirement {
	case zipperv1.MemoRequirement_REQUIRED:
		return models.MemoRequirementRequired
	case zipperv1.MemoRequirement_OPTIONAL:
		return models.MemoRequirementOptional
	case zipperv1.MemoRequirement_NOT_SUPPORTED:
		return models.MemoRequirementNotSupported
	case zipperv1.MemoRequirement_MEMO_UNSPECIFIED:
		return ""
	default:
		return fmt.Sprintf("UNKNOWN(%d)", requirement)
	}
}

func zippedAssetSupplyUpdateFromProto(msg *zipperv1.ZippedAssetSupplyUpdate, scaleFn func(uint32) (int, bool)) (models.ZippedAssetSupplyUpdate, error) {
	if msg == nil {
		return models.ZippedAssetSupplyUpdate{}, &errors.ValidationError{Msg: "zipped asset supply update is missing"}
	}
	if scaleFn == nil {
		return models.ZippedAssetSupplyUpdate{}, &errors.ValidationError{Msg: "zipped asset supply decoding requires a hydrated catalog scale"}
	}
	scale, ok := scaleFn(msg.GetZippedAssetId())
	if !ok {
		return models.ZippedAssetSupplyUpdate{}, &errors.ValidationError{Msg: "unknown quantity scale for zipped_asset_id"}
	}
	update := models.ZippedAssetSupplyUpdate{
		ZippedAssetID: msg.GetZippedAssetId(),
		Status:        supplyStatusLabel(msg.GetStatus()),
		ObservedAt:    timestampTime(msg.GetObservedAt()),
	}
	// Updates carry AVAILABLE or STALE; any other status names no known
	// supply, as the contract reads an unspecified one: never zero.
	if supplyKnown(msg.GetStatus()) {
		if msg.GetObservedAt() == nil {
			return models.ZippedAssetSupplyUpdate{}, &errors.ResponseContractError{
				Operation: "zipped asset supply update",
				Msg:       fmt.Sprintf("zipped asset %d has a known supply without observed_at", msg.GetZippedAssetId()),
			}
		}
		update.Supply = formatLedgerU128OrZero(strconv.FormatUint(msg.GetSupplyQ(), 10), scale)
	}
	return update, nil
}

// ZippedAssetSupplyBatchFromProto decodes a realtime supply batch.
func ZippedAssetSupplyBatchFromProto(msg *zipperv1.ZippedAssetSupplyBatch, scaleFn func(uint32) (int, bool)) (models.ZippedAssetSupplyBatch, error) {
	out := make([]models.ZippedAssetSupplyUpdate, 0, len(msg.GetUpdates()))
	for _, u := range msg.GetUpdates() {
		update, err := zippedAssetSupplyUpdateFromProto(u, scaleFn)
		if err != nil {
			return models.ZippedAssetSupplyBatch{}, err
		}
		out = append(out, update)
	}
	return models.ZippedAssetSupplyBatch{Updates: out}, nil
}
