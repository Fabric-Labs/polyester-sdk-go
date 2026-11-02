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
)

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
}

// ChangelogSection groups a post's changes under one heading.
type ChangelogSection struct {
	Heading string            `json:"heading"`
	Summary *string           `json:"summary,omitempty"`
	Changes []ChangelogChange `json:"changes"`
}

// ChangelogPostSummary is a changelog post without its sections, with change
// counts by kind.
type ChangelogPostSummary struct {
	// Slug is the post's coverage cutoff date, YYYY-MM-DD, with a -2 or -3
	// suffix for a later post on the same day.
	Slug                string     `json:"slug"`
	Title               string     `json:"title"`
	Introduction        string     `json:"introduction"`
	PublishedAt         *time.Time `json:"published_at,omitempty"`
	CoverageCutoff      *time.Time `json:"coverage_cutoff,omitempty"`
	ChangeCount         uint32     `json:"change_count"`
	BreakingChangeCount uint32     `json:"breaking_change_count"`
	ActionRequiredCount uint32     `json:"action_required_count"`
	NewFeatureCount     uint32     `json:"new_feature_count"`
	ImprovementCount    uint32     `json:"improvement_count"`
	BugFixCount         uint32     `json:"bug_fix_count"`
}

// ChangelogPost is one full changelog post.
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
	// Surfaces are ChangelogSurface* labels.
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
