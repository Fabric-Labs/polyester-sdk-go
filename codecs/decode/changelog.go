package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// ChangelogChangeFromProto decodes one change. The id becomes base58; enum
// values newer than this SDK read as "unknown(<n>)".
func ChangelogChangeFromProto(msg *changelogv1.Change) models.ChangelogChange {
	if msg == nil {
		return models.ChangelogChange{Audiences: []string{}, Surfaces: []string{}}
	}
	audiences := make([]string, 0, len(msg.GetAudiences()))
	for _, audience := range msg.GetAudiences() {
		if label := codecs.ProtoEnumName(audience, audience.Number()); label != "" {
			audiences = append(audiences, label)
		}
	}
	surfaces := make([]string, 0, len(msg.GetSurfaces()))
	for _, surface := range msg.GetSurfaces() {
		if label := codecs.ProtoEnumName(surface, surface.Number()); label != "" {
			surfaces = append(surfaces, label)
		}
	}
	category := msg.GetCategory()
	return models.ChangelogChange{
		ID:                codecs.FormatUint64ID(msg.GetId()),
		Title:             msg.GetTitle(),
		Summary:           msg.GetSummary(),
		Details:           cloneOptionalString(msg.Details),
		Category:          codecs.ProtoEnumName(category, category.Number()),
		ActionRequired:    msg.GetActionRequired(),
		MigrationGuidance: cloneOptionalString(msg.MigrationGuidance),
		Audiences:         audiences,
		Surfaces:          surfaces,
	}
}

// ChangelogPostSummaryFromProto decodes a post summary.
func ChangelogPostSummaryFromProto(msg *changelogv1.PostSummary) models.ChangelogPostSummary {
	if msg == nil {
		return models.ChangelogPostSummary{}
	}
	return models.ChangelogPostSummary{
		Slug:                msg.GetSlug(),
		Title:               msg.GetTitle(),
		Introduction:        msg.GetIntroduction(),
		PublishedAt:         timestampTime(msg.GetPublishedAt()),
		CoverageCutoff:      timestampTime(msg.GetCoverageCutoff()),
		ChangeCount:         msg.GetChangeCount(),
		BreakingChangeCount: msg.GetBreakingChangeCount(),
		ActionRequiredCount: msg.GetActionRequiredCount(),
		NewFeatureCount:     msg.GetNewFeatureCount(),
		ImprovementCount:    msg.GetImprovementCount(),
		BugFixCount:         msg.GetBugFixCount(),
	}
}

// ChangelogPostsPageFromProto decodes ListPostsResponse.
func ChangelogPostsPageFromProto(msg *changelogv1.ListPostsResponse) models.ChangelogPostsPage {
	posts := make([]models.ChangelogPostSummary, 0, len(msg.GetPosts()))
	for _, post := range msg.GetPosts() {
		if post == nil {
			continue
		}
		posts = append(posts, ChangelogPostSummaryFromProto(post))
	}
	return models.ChangelogPostsPage{Posts: posts, NextPageToken: msg.GetNextPageToken()}
}

// ChangelogPostFromProto decodes GetPostResponse. A success without a post
// is a contract violation.
func ChangelogPostFromProto(msg *changelogv1.GetPostResponse) (models.ChangelogPost, error) {
	post := msg.GetPost()
	if post == nil || post.GetSummary() == nil {
		return models.ChangelogPost{}, &sdkerrors.ResponseContractError{Operation: "GetPost", Msg: "missing post"}
	}
	sections := make([]models.ChangelogSection, 0, len(post.GetSections()))
	for _, section := range post.GetSections() {
		if section == nil {
			continue
		}
		changes := make([]models.ChangelogChange, 0, len(section.GetChanges()))
		for _, change := range section.GetChanges() {
			if change == nil {
				continue
			}
			changes = append(changes, ChangelogChangeFromProto(change))
		}
		sections = append(sections, models.ChangelogSection{
			Heading: section.GetHeading(),
			Summary: cloneOptionalString(section.Summary),
			Changes: changes,
		})
	}
	return models.ChangelogPost{
		ChangelogPostSummary: ChangelogPostSummaryFromProto(post.GetSummary()),
		Sections:             sections,
	}, nil
}

// ChangelogChangesPageFromProto decodes ListChangesResponse.
func ChangelogChangesPageFromProto(msg *changelogv1.ListChangesResponse) models.ChangelogChangesPage {
	entries := make([]models.ChangelogChangeEntry, 0, len(msg.GetChanges()))
	for _, entry := range msg.GetChanges() {
		if entry == nil {
			continue
		}
		entries = append(entries, models.ChangelogChangeEntry{
			Change:         ChangelogChangeFromProto(entry.GetChange()),
			PostSlug:       entry.GetPostSlug(),
			PublishedAt:    timestampTime(entry.GetPublishedAt()),
			SectionHeading: entry.GetSectionHeading(),
		})
	}
	return models.ChangelogChangesPage{Changes: entries, NextPageToken: msg.GetNextPageToken()}
}
