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
	// EditorialUpdatedAt is the last effective change to Editorial; nil for
	// an asset listed only for its MarketStats, whose Editorial is empty.
	EditorialUpdatedAt *time.Time `json:"editorial_updated_at,omitempty"`
	// MarketStats holds the asset's market datapoints; nil when the asset has
	// none.
	MarketStats *AssetMarketStats `json:"market_stats,omitempty"`
}

// AssetMarketStats are an asset's market datapoints. A nil value is unknown:
// hide its row.
type AssetMarketStats struct {
	// CirculatingSupply, TotalSupply and MaxSupply are in asset units.
	CirculatingSupply *AssetMarketValue `json:"circulating_supply,omitempty"`
	TotalSupply       *AssetMarketValue `json:"total_supply,omitempty"`
	MaxSupply         *AssetMarketValue `json:"max_supply,omitempty"`
	// MaxSupplyUnlimited is set, and MaxSupply nil, when the source reports
	// no maximum supply. When both are nil the maximum is unknown.
	MaxSupplyUnlimited *AssetMarketFlag `json:"max_supply_unlimited,omitempty"`
	// MarketCapUSD and FullyDilutedMarketCapUSD are in US dollars.
	MarketCapUSD             *AssetMarketValue `json:"market_cap_usd,omitempty"`
	FullyDilutedMarketCapUSD *AssetMarketValue `json:"fully_diluted_market_cap_usd,omitempty"`
	// Rank is the market capitalization rank, a positive integer where 1 is
	// the largest.
	Rank *AssetMarketValue `json:"rank,omitempty"`
}

// AssetMarketValue is one market datapoint with its source and the time the
// source observed or set it.
type AssetMarketValue struct {
	// Value is a nonnegative decimal string without sign or exponent, kept as
	// sent; never parse it as a float.
	Value string `json:"value"`
	// Source is "coinmarketcap" (show "Source: CoinMarketCap" with the value)
	// or "polyester" (set by the Polyester team); "unknown(n)" for a source
	// this SDK does not know.
	Source string `json:"source,omitempty"`
	// AsOf is when the source last observed or set the value. Values stay
	// until a newer observation replaces them, so show the time with the
	// value.
	AsOf *time.Time `json:"as_of,omitempty"`
}

// AssetMarketFlag is a market fact that holds, with its source and the time
// the source observed it.
type AssetMarketFlag struct {
	Source string     `json:"source,omitempty"`
	AsOf   *time.Time `json:"as_of,omitempty"`
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
