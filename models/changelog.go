package models

import "time"

// Changelog change category labels. A category newer than this SDK reads as
// "unknown(<n>)".
const (
	ChangelogCategoryBreakingChange = "breaking_change"
	ChangelogCategoryNewFeature     = "new_feature"
	ChangelogCategoryImprovement    = "improvement"
	ChangelogCategoryBugFix         = "bug_fix"
)

// Changelog audience labels. An audience newer than this SDK reads as
// "unknown(<n>)".
const (
	ChangelogAudienceAllUsers     = "all_users"
	ChangelogAudienceTraders      = "traders"
	ChangelogAudienceMarketMakers = "market_makers"
	ChangelogAudienceDevelopers   = "developers"
)

// Changelog surface labels. A surface newer than this SDK reads as
// "unknown(<n>)".
const (
	ChangelogSurfaceTrading       = "trading"
	ChangelogSurfaceFunding       = "funding"
	ChangelogSurfaceAccounts      = "accounts"
	ChangelogSurfaceAPI           = "api"
	ChangelogSurfaceExplorer      = "explorer"
	ChangelogSurfaceDocumentation = "documentation"
	ChangelogSurfacePlatform      = "platform"
	// ChangelogSurfaceMarketData covers prices, candles, the order book,
	// heatmaps and market overviews.
	ChangelogSurfaceMarketData = "market_data"
	// ChangelogSurfaceQuickSwap covers the QuickSwap swap app.
	ChangelogSurfaceQuickSwap = "quickswap"
	// ChangelogSurfaceRewards covers daily claims, campaigns, VIP tiers and
	// fee discounts.
	ChangelogSurfaceRewards = "rewards"
	// ChangelogSurfaceAssistant covers the AI assistant: chats, approvals,
	// memories and preferences.
	ChangelogSurfaceAssistant = "assistant"
)

// Changelog environment labels. An environment newer than this SDK reads as
// "unknown(<n>)".
const (
	ChangelogEnvironmentDevnet  = "devnet"
	ChangelogEnvironmentTestnet = "testnet"
	ChangelogEnvironmentMainnet = "mainnet"
)

// Changelog change availability labels: how one change of a release differs
// from its release in one environment. An availability newer than this SDK
// reads as "unknown(<n>)".
const (
	// ChangelogAvailabilityAhead: the change is available in the environment
	// while its release is still pending there.
	ChangelogAvailabilityAhead = "ahead"
	// ChangelogAvailabilityPending: the change is not yet available in the
	// environment although its release is live there.
	ChangelogAvailabilityPending = "pending"
	// ChangelogAvailabilityUnavailable: the change is not available in the
	// environment for now, whatever its release's status.
	ChangelogAvailabilityUnavailable = "unavailable"
)

// ChangelogEnvironmentStatus says whether a release is live in one
// environment.
type ChangelogEnvironmentStatus struct {
	// Environment is a ChangelogEnvironment* label.
	Environment string `json:"environment"`
	// LiveAt is when the release went live in the environment; nil while it
	// is pending there.
	LiveAt *time.Time `json:"live_at,omitempty"`
}

// ChangelogChangeException is the status of one change in one environment
// where it differs from its release's status.
type ChangelogChangeException struct {
	// Environment is a ChangelogEnvironment* label.
	Environment string `json:"environment"`
	// Availability is a ChangelogAvailability* label.
	Availability string `json:"availability"`
	// LiveAt is when the change became available in the environment; set
	// for ChangelogAvailabilityAhead only, nil otherwise.
	LiveAt *time.Time `json:"live_at,omitempty"`
}

// ChangelogChange is one product change in a changelog post.
type ChangelogChange struct {
	// ID is the change's public identifier in base58.
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	Details           *string  `json:"details,omitempty"`
	Category          string   `json:"category"`
	ActionRequired    bool     `json:"action_required"`
	MigrationGuidance *string  `json:"migration_guidance,omitempty"`
	Audiences         []string `json:"audiences"`
	Surfaces          []string `json:"surfaces"`
	// EnvironmentExceptions are the environments, any of them and not only
	// the one the client points at, where this change differs from its
	// release's status, for example a change already available on testnet
	// before its release reaches testnet. Empty outside release posts and
	// when the change follows its release everywhere.
	EnvironmentExceptions []ChangelogChangeException `json:"environment_exceptions,omitempty"`
}

// ChangelogSection groups a post's changes under one heading.
type ChangelogSection struct {
	Heading string `json:"heading"`
	// Surface is the product area the section covers, a ChangelogSurface*
	// label; empty for a section tied to no one area, such as the changes
	// that need your attention. It does not change which changes
	// ListChanges matches: filter on ChangelogChange.Surfaces.
	Surface string            `json:"surface,omitempty"`
	Summary *string           `json:"summary,omitempty"`
	Changes []ChangelogChange `json:"changes"`
}

// ChangelogPostSummary is a changelog post without its sections, with change
// counts by kind.
//
// A published post can be republished, for example to correct its copy or
// withdraw a change. Its Slug, PublishedAt and its changes' IDs stay the same;
// Revision and UpdatedAt tell the versions apart. To follow edits, compare
// Revision with the one you hold and read the post again when it differs.
type ChangelogPostSummary struct {
	// Slug is the post's coverage cutoff date, YYYY-MM-DD, with a -2 or -3
	// suffix for a later post on the same day. It never changes when the post
	// is republished.
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Introduction string `json:"introduction"`
	// PublishedAt is the time of first publication. A republication does not
	// move it.
	PublishedAt *time.Time `json:"published_at,omitempty"`
	// UpdatedAt is when the current revision was published; it equals
	// PublishedAt for revision 1. Nil from a server that predates revisions.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// Revision is the published revision, starting at 1 and increasing with
	// each republication. 0 from a server that predates revisions.
	Revision            uint32     `json:"revision"`
	CoverageCutoff      *time.Time `json:"coverage_cutoff,omitempty"`
	ChangeCount         uint32     `json:"change_count"`
	BreakingChangeCount uint32     `json:"breaking_change_count"`
	ActionRequiredCount uint32     `json:"action_required_count"`
	NewFeatureCount     uint32     `json:"new_feature_count"`
	ImprovementCount    uint32     `json:"improvement_count"`
	BugFixCount         uint32     `json:"bug_fix_count"`
	// ReleaseVersion is the release the post announces, for example
	// "0.92.9"; nil for a post that does not announce one release. A
	// release post has the same slug ("v0.92.9") and content in every
	// environment, and is listed even where its release is still pending.
	ReleaseVersion *string `json:"release_version,omitempty"`
	// Environments is the status of the post's release in each environment,
	// devnet, testnet and mainnet in that order; empty for a post without
	// ReleaseVersion. It changes when the release goes live somewhere,
	// without a new Revision: compare the whole summary to notice it.
	Environments []ChangelogEnvironmentStatus `json:"environments,omitempty"`
	// DeploymentGroupSlug is the slug of the deployment group that brought
	// the release to this environment, for Changelog.GetDeploymentGroup; nil
	// while the release is pending here and for a post without
	// ReleaseVersion.
	DeploymentGroupSlug *string `json:"deployment_group_slug,omitempty"`
}

// ChangelogPost is one full changelog post at its current revision.
type ChangelogPost struct {
	ChangelogPostSummary
	Sections []ChangelogSection `json:"sections"`
}

// ChangelogChangeEntry is one change with the post it was published in.
type ChangelogChangeEntry struct {
	Change         ChangelogChange `json:"change"`
	PostSlug       string          `json:"post_slug"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
	SectionHeading string          `json:"section_heading"`
}

// ChangelogPostsPage is one ListPosts page, newest post first.
type ChangelogPostsPage struct {
	Posts         []ChangelogPostSummary `json:"posts"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
}

// ChangelogChangesPage is one ListChanges page.
type ChangelogChangesPage struct {
	Changes       []ChangelogChangeEntry `json:"changes"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
}

// ListChangelogPostsOptions pages ListPosts. Zero Limit uses the server
// default of 20 (maximum 100).
type ListChangelogPostsOptions struct {
	Limit     int
	PageToken string
}

// ListChangelogChangesOptions filters ListChanges. Values within one filter
// combine with OR; different filters combine with AND. Zero values send
// nothing.
type ListChangelogChangesOptions struct {
	// Categories are ChangelogCategory* labels.
	Categories []string
	// ActionRequired keeps only changes whose action_required matches.
	ActionRequired *bool
	// Surfaces are ChangelogSurface* labels, at most 11.
	Surfaces []string
	// Audiences are ChangelogAudience* labels. Changes addressed to all users
	// always match.
	Audiences []string
	// StartTime (inclusive) and EndTime (exclusive) bound post publication
	// time; EndTime must be after StartTime when both are set.
	StartTime *time.Time
	EndTime   *time.Time
	// PostSlug restricts the result to one post.
	PostSlug string
	// Sort is "desc" (newest post first, the default) or "asc". Changes keep
	// publication order inside each post.
	Sort string
	// Limit is the page size; zero uses the server default of 100 (maximum
	// 500).
	Limit int
	// PageToken continues a previous page; reuse it only with the same
	// filters, time bounds and sort.
	PageToken string
}

// ChangelogDeploymentGroupPost is one published release post a deployment
// group brought.
type ChangelogDeploymentGroupPost struct {
	// Slug is the post's slug, for example "v0.92.9", for Changelog.GetPost.
	Slug string `json:"slug"`
	// ReleaseVersion is the release the post announces, for example "0.92.9".
	ReleaseVersion string `json:"release_version"`
	Title          string `json:"title"`
	// PublishedAt is the time of first publication of the post.
	PublishedAt *time.Time `json:"published_at,omitempty"`
	// Revision is the post's published revision, starting at 1.
	Revision uint32 `json:"revision"`
	// UpdatedAt is when the post's current revision was published.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// ChangelogDeploymentGroupSummary is one deployment group without its
// details: one deployment to the environment that raised its release
// version, bringing every release above the previous group's version up to
// and including its own.
type ChangelogDeploymentGroupSummary struct {
	// Slug is the group's stable, shareable identifier, the environment and
	// the group's own release, for example "testnet-v0.92.9".
	Slug string `json:"slug"`
	// Environment is the ChangelogEnvironment* label the group was deployed
	// to.
	Environment string `json:"environment"`
	// OccurredAt is when the deployment ran.
	OccurredAt *time.Time `json:"occurred_at,omitempty"`
	// ReleaseVersion is the group's own, highest release, for example
	// "0.92.9".
	ReleaseVersion string `json:"release_version"`
	// ReleaseTag is the tag of the group's own release, for example
	// "v0.92.9"; nil when unknown.
	ReleaseTag *string `json:"release_tag,omitempty"`
	// PreviousReleaseVersion is the release of the previous deployment
	// group; nil for the first group.
	PreviousReleaseVersion *string `json:"previous_release_version,omitempty"`
	// Versions is every release the group brought, ascending, for example
	// ["0.92.8", "0.92.9"].
	Versions []string `json:"versions"`
	// Posts are the published release posts of those releases, ascending by
	// release. A release without a published post has none.
	Posts []ChangelogDeploymentGroupPost `json:"posts"`
}

// ChangelogDeploymentGroupAttention is one change of a deployment group that
// needs attention: breaking, or with ActionRequired.
type ChangelogDeploymentGroupAttention struct {
	// PostSlug is the slug of the release post that announces the change.
	PostSlug string `json:"post_slug"`
	// ReleaseVersion is the release of that post.
	ReleaseVersion string `json:"release_version"`
	// Change is the change as in its post.
	Change ChangelogChange `json:"change"`
}

// ChangelogDeploymentGroupHighlight is one highlight of a release post in a
// deployment group.
type ChangelogDeploymentGroupHighlight struct {
	// PostSlug is the slug of the release post.
	PostSlug string `json:"post_slug"`
	// ReleaseVersion is the release of that post.
	ReleaseVersion string `json:"release_version"`
	// Text is the highlight, inline Markdown.
	Text string `json:"text"`
}

// ChangelogDeploymentGroup is one complete deployment group.
type ChangelogDeploymentGroup struct {
	ChangelogDeploymentGroupSummary
	// Attention lists the breaking changes and the changes that require
	// action across the group's posts, in post order (ascending release),
	// then section and change order.
	Attention []ChangelogDeploymentGroupAttention `json:"attention"`
	// Highlights are the highlights of the group's posts, in post order.
	Highlights []ChangelogDeploymentGroupHighlight `json:"highlights"`
}

// ChangelogDeploymentGroupsPage is one ListDeploymentGroups page, newest
// deployment first.
type ChangelogDeploymentGroupsPage struct {
	Groups        []ChangelogDeploymentGroupSummary `json:"groups"`
	NextPageToken string                            `json:"next_page_token,omitempty"`
}

// ListChangelogDeploymentGroupsOptions pages ListDeploymentGroups. Zero Limit
// uses the server default of 20 (maximum 100).
type ListChangelogDeploymentGroupsOptions struct {
	Limit int
	// PageToken continues a previous page: an opaque cursor bound to
	// deployment time and slug.
	PageToken string
}
