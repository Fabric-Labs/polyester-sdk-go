package decode

import (
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// AssetInfoListFromProto decodes a ListAssetInfo snapshot in the server's
// ascending asset ID order; empty, never nil.
func AssetInfoListFromProto(msg *marketdatav1.ListAssetInfoResponse) []models.AssetInfo {
	out := make([]models.AssetInfo, 0, len(msg.GetAssetInfo()))
	for _, info := range msg.GetAssetInfo() {
		if info == nil {
			continue
		}
		out = append(out, models.AssetInfo{
			AssetID:            info.GetAssetId(),
			Editorial:          assetEditorialInfo(info.GetEditorial()),
			EditorialUpdatedAt: timestampTime(info.GetEditorialUpdatedAt()),
			MarketStats:        assetMarketStats(info.GetMarketStats()),
		})
	}
	return out
}

func assetMarketStats(msg *marketdatav1.AssetMarketStats) *models.AssetMarketStats {
	if msg == nil {
		return nil
	}
	out := &models.AssetMarketStats{
		CirculatingSupply:        assetMarketValue(msg.GetCirculatingSupply()),
		TotalSupply:              assetMarketValue(msg.GetTotalSupply()),
		MaxSupply:                assetMarketValue(msg.GetMaxSupply()),
		MarketCapUSD:             assetMarketValue(msg.GetMarketCapUsd()),
		FullyDilutedMarketCapUSD: assetMarketValue(msg.GetFullyDilutedMarketCapUsd()),
		Rank:                     assetMarketValue(msg.GetRank()),
	}
	// A presence-only fact: present means the source reports no maximum.
	if flag := msg.GetMaxSupplyUnlimited(); flag != nil {
		out.MaxSupplyUnlimited = &models.AssetMarketFlag{Source: enumLabel(flag.GetSource(), ""), AsOf: timestampTime(flag.GetAsOf())}
	}
	return out
}

// assetMarketValue decodes one datapoint; nil when absent, which is unknown.
func assetMarketValue(msg *marketdatav1.AssetMarketValue) *models.AssetMarketValue {
	if msg == nil {
		return nil
	}
	return &models.AssetMarketValue{Value: msg.GetValue(), Source: enumLabel(msg.GetSource(), ""), AsOf: timestampTime(msg.GetAsOf())}
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
