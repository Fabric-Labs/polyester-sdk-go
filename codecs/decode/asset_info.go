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
		})
	}
	return out
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
