package models

import "time"

// AssetInfo is the team-maintained information about one asset, as
// MarketData.ListAssetInfo returns it.
type AssetInfo struct {
	// AssetID is the numeric asset ID, the LedgerID of the asset in the spot
	// and funding configs.
	AssetID uint32 `json:"asset_id"`
	// Editorial holds the asset's description, official links and launch
	// facts.
	Editorial AssetEditorialInfo `json:"editorial"`
	// EditorialUpdatedAt is the last effective change to Editorial.
	EditorialUpdatedAt *time.Time `json:"editorial_updated_at,omitempty"`
}

// AssetEditorialInfo is plain text and official links about an asset. An
// empty string or list, or a nil IssuePrice, means unknown: hide the row.
type AssetEditorialInfo struct {
	// Description is plain text, at most 10000 characters; render it as text.
	Description string `json:"description,omitempty"`
	// WebsiteURL is the official HTTPS website.
	WebsiteURL string `json:"website_url,omitempty"`
	// WhitepaperURL is the HTTPS whitepaper.
	WhitepaperURL string `json:"whitepaper_url,omitempty"`
	// ExplorerURLs are up to 10 HTTPS block explorers, in editorial order.
	ExplorerURLs []string `json:"explorer_urls,omitempty"`
	// XURL is the official HTTPS X profile.
	XURL string `json:"x_url,omitempty"`
	// TelegramURL is the official HTTPS Telegram.
	TelegramURL string `json:"telegram_url,omitempty"`
	// DiscordURL is the official HTTPS Discord.
	DiscordURL string `json:"discord_url,omitempty"`
	// GitHubURL is the official HTTPS GitHub.
	GitHubURL string `json:"github_url,omitempty"`
	// IssueDate is the launch calendar date as YYYY-MM-DD, kept as sent; ""
	// when unknown. It has no time zone.
	IssueDate string `json:"issue_date,omitempty"`
	// IssuePrice is the launch price; nil when unknown. A known zero price is
	// an IssuePrice with Amount "0".
	IssuePrice *AssetIssuePrice `json:"issue_price,omitempty"`
}

// AssetIssuePrice is an asset's launch price.
type AssetIssuePrice struct {
	// Amount is a nonnegative exact decimal string, kept as sent.
	Amount string `json:"amount"`
	// Currency is always "USD".
	Currency string `json:"currency"`
}
