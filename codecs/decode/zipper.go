package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func DepositWithdrawConfigFromProto(msg *zipperv1.GetDepositWithdrawConfigResponse) models.DepositWithdrawConfig {
	cfg := models.DepositWithdrawConfig{
		PolyesterChainID: msg.GetPolyesterChainId(),
		TsMs:             int64(msg.GetTsSec()) * 1000,
	}
	for _, c := range msg.GetChains() {
		cfg.Chains = append(cfg.Chains, models.ZipperChainConfig{
			ChainID: c.GetChainId(), Code: c.GetCode(), Name: c.GetName(), NativeChainID: c.GetNativeChainId(),
			NativeCurrencySymbol: c.GetNativeCurrencySymbol(), ExplorerURL: c.GetExplorerUrl(), Icon: c.GetIcon(),
			RequiredConfirmations: int(c.GetRequiredConfirmations()), ConfirmationTimeSeconds: int(c.GetConfirmationTimeSeconds()),
			IsCaseSensitive: c.GetIsCaseSensitive(), MinAddressLength: int(c.GetMinAddressLength()), MaxAddressLength: int(c.GetMaxAddressLength()),
		})
	}
	for _, a := range msg.GetAssets() {
		asset := models.ZipperAssetConfig{
			Asset: a.GetAsset(), LedgerID: a.GetLedgerId(), Name: a.GetName(), Icon: a.GetIcon(),
			QuantityScale: int(a.GetQuantityScale()), QuantityDisplayDecimals: int(a.GetQuantityDisplayDecimals()), UAssetID: a.GetUAssetId(),
		}
		for _, v := range a.GetVariants() {
			asset.Variants = append(asset.Variants, models.ZipperAssetChainVariant{
				ZippedAssetID: v.GetZippedAssetId(), ChainID: v.GetChainId(), IsNativeAsset: v.GetIsNativeAsset(),
				NetworkFee: v.GetNetworkFee(), DepositMinAmount: v.GetDepositMinAmount(), WithdrawMinAmount: v.GetWithdrawMinAmount(),
				SourceToken: models.ZipperTokenConfig{Address: v.GetSourceAddress(), Decimals: int(v.GetSourceDecimals())},
				ZToken:      models.ZipperTokenConfig{Address: v.GetZtokenAddress(), Decimals: int(v.GetZtokenDecimals())},
			})
		}
		cfg.Assets = append(cfg.Assets, asset)
	}
	for _, c := range msg.GetContracts() {
		cfg.Contracts = append(cfg.Contracts, models.ZipperChainContractConfig{
			Name: c.GetName(), Address: c.GetAddress(), Type: c.GetType(),
			Description: c.GetDescription(), Version: int(c.GetVersion()),
		})
	}
	return cfg
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
	return models.ZippedAssetSupplyUpdate{
		ZippedAssetID: msg.GetZippedAssetId(),
		Supply:        formatLedgerU128OrZero(strconv.FormatUint(msg.GetSupplyQ(), 10), scale),
	}, nil
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
