package decode

import (
	"fmt"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// AssetInfoListFromProto decodes a ListAssetInfo snapshot in the server's
// ascending asset ID order; empty, never nil.
// A market datapoint that is present without its value, source or time, or
// a presence-only fact without its source or time, returns a
// *errors.ResponseContractError; an absent one is unknown.
func AssetInfoListFromProto(msg *marketdatav1.ListAssetInfoResponse) ([]models.AssetInfo, error) {
	out := make([]models.AssetInfo, 0, len(msg.GetAssetInfo()))
	for _, info := range msg.GetAssetInfo() {
		if info == nil {
			continue
		}
		stats, err := assetMarketStats(info.GetAssetId(), info.GetMarketStats())
		if err != nil {
			return nil, err
		}
		out = append(out, models.AssetInfo{
			AssetID:            info.GetAssetId(),
			Editorial:          assetEditorialInfo(info.GetEditorial()),
			EditorialUpdatedAt: timestampTime(info.GetEditorialUpdatedAt()),
			MarketStats:        stats,
		})
	}
	return out, nil
}

func assetMarketStats(assetID uint32, msg *marketdatav1.AssetMarketStats) (*models.AssetMarketStats, error) {
	if msg == nil {
		return nil, nil
	}
	out := &models.AssetMarketStats{}
	for _, field := range []struct {
		name string
		in   *marketdatav1.AssetMarketValue
		out  **models.AssetMarketValue
	}{
		{"circulating_supply", msg.GetCirculatingSupply(), &out.CirculatingSupply},
		{"total_supply", msg.GetTotalSupply(), &out.TotalSupply},
		{"max_supply", msg.GetMaxSupply(), &out.MaxSupply},
		{"market_cap_usd", msg.GetMarketCapUsd(), &out.MarketCapUSD},
		{"fully_diluted_market_cap_usd", msg.GetFullyDilutedMarketCapUsd(), &out.FullyDilutedMarketCapUSD},
		{"rank", msg.GetRank(), &out.Rank},
	} {
		value, err := assetMarketValue(assetID, field.name, field.in)
		if err != nil {
			return nil, err
		}
		*field.out = value
	}
	// A presence-only fact: present means the source reports no maximum.
	if flag := msg.GetMaxSupplyUnlimited(); flag != nil {
		if flag.GetSource() == marketdatav1.AssetDataSource_SOURCE_UNSPECIFIED || flag.GetAsOf() == nil {
			return nil, assetMarketError(assetID, "max_supply_unlimited", "source and as_of")
		}
		out.MaxSupplyUnlimited = &models.AssetMarketFlag{Source: enumLabel(flag.GetSource(), ""), AsOf: timestampTime(flag.GetAsOf())}
	}
	return out, nil
}

// assetMarketValue decodes one datapoint: nil when absent, which is unknown,
// and a *errors.ResponseContractError when present without its value,
// source or time.
func assetMarketValue(assetID uint32, name string, msg *marketdatav1.AssetMarketValue) (*models.AssetMarketValue, error) {
	if msg == nil {
		return nil, nil
	}
	if msg.GetValue() == "" || msg.GetSource() == marketdatav1.AssetDataSource_SOURCE_UNSPECIFIED || msg.GetAsOf() == nil {
		return nil, assetMarketError(assetID, name, "value, source and as_of")
	}
	return &models.AssetMarketValue{Value: msg.GetValue(), Source: enumLabel(msg.GetSource(), ""), AsOf: timestampTime(msg.GetAsOf())}, nil
}

func assetMarketError(assetID uint32, name, parts string) error {
	return &sdkerrors.ResponseContractError{Operation: "ListAssetInfo", Msg: fmt.Sprintf("asset %d %s lacks its %s", assetID, name, parts)}
}

func assetEditorialInfo(msg *marketdatav1.AssetEditorialInfo) models.AssetEditorialInfo {
	out := models.AssetEditorialInfo{
		Description:   msg.GetDescription(),
		WebsiteURL:    msg.GetWebsiteUrl(),
		WhitepaperURL: msg.GetWhitepaperUrl(),
		ExplorerURLs:  cloneStrings(msg.GetExplorerUrls()),
		XURL:          msg.GetXUrl(),
		TelegramURL:   msg.GetTelegramUrl(),
		DiscordURL:    msg.GetDiscordUrl(),
		GitHubURL:     msg.GetGithubUrl(),
		IssueDate:     msg.GetIssueDate(),
	}
	// An absent price is unknown; a present one, zero included, is known.
	if price := msg.GetIssuePrice(); price != nil {
		out.IssuePrice = &models.AssetIssuePrice{Amount: price.GetAmount(), Currency: price.GetCurrency()}
	}
	return out
}
