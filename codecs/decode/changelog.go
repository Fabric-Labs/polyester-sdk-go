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
		ID:                    codecs.FormatUint64ID(msg.GetId()),
		Title:                 msg.GetTitle(),
		Summary:               msg.GetSummary(),
		Details:               cloneOptionalString(msg.Details),
		Category:              codecs.ProtoEnumName(category, category.Number()),
		ActionRequired:        msg.GetActionRequired(),
		MigrationGuidance:     cloneOptionalString(msg.MigrationGuidance),
		Audiences:             audiences,
		Surfaces:              surfaces,
		EnvironmentExceptions: changelogChangeExceptions(msg.GetEnvironmentExceptions()),
	}
}

// changelogChangeExceptions decodes a change's environment exceptions; nil
// when it has none.
func changelogChangeExceptions(msgs []*changelogv1.ChangeException) []models.ChangelogChangeException {
	var out []models.ChangelogChangeException
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		availability := msg.GetAvailability()
		out = append(out, models.ChangelogChangeException{
			Environment:  changelogEnvironmentLabel(msg.GetEnvironment()),
			Availability: codecs.ProtoEnumName(availability, availability.Number()),
			LiveAt:       timestampTime(msg.GetLiveAt()),
		})
	}
	return out
}

func changelogEnvironmentLabel(environment changelogv1.Environment) string {
	return codecs.ProtoEnumName(environment, environment.Number())
}

// changelogEnvironments decodes a release post's per-environment status; nil
// for a post without a release.
func changelogEnvironments(msgs []*changelogv1.EnvironmentStatus) []models.ChangelogEnvironmentStatus {
	var out []models.ChangelogEnvironmentStatus
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		out = append(out, models.ChangelogEnvironmentStatus{
			Environment: changelogEnvironmentLabel(msg.GetEnvironment()),
			LiveAt:      timestampTime(msg.GetLiveAt()),
		})
	}
	return out
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
		UpdatedAt:           timestampTime(msg.GetUpdatedAt()),
		Revision:            msg.GetRevision(),
		CoverageCutoff:      timestampTime(msg.GetCoverageCutoff()),
		ChangeCount:         msg.GetChangeCount(),
		BreakingChangeCount: msg.GetBreakingChangeCount(),
		ActionRequiredCount: msg.GetActionRequiredCount(),
		NewFeatureCount:     msg.GetNewFeatureCount(),
		ImprovementCount:    msg.GetImprovementCount(),
		BugFixCount:         msg.GetBugFixCount(),
		ReleaseVersion:      cloneOptionalString(msg.ReleaseVersion),
		Environments:        changelogEnvironments(msg.GetEnvironments()),
		DeploymentGroupSlug: cloneOptionalString(msg.DeploymentGroupSlug),
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
			Surface: codecs.ProtoEnumName(section.GetSurface(), section.GetSurface().Number()),
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

// ChangelogDeploymentGroupSummaryFromProto decodes a deployment group summary.
func ChangelogDeploymentGroupSummaryFromProto(msg *changelogv1.DeploymentGroupSummary) models.ChangelogDeploymentGroupSummary {
	versions := append([]string{}, msg.GetVersions()...)
	posts := make([]models.ChangelogDeploymentGroupPost, 0, len(msg.GetPosts()))
	for _, post := range msg.GetPosts() {
		if post == nil {
			continue
		}
		posts = append(posts, models.ChangelogDeploymentGroupPost{
			Slug:           post.GetSlug(),
			ReleaseVersion: post.GetReleaseVersion(),
			Title:          post.GetTitle(),
			PublishedAt:    timestampTime(post.GetPublishedAt()),
			Revision:       post.GetRevision(),
			UpdatedAt:      timestampTime(post.GetUpdatedAt()),
		})
	}
	return models.ChangelogDeploymentGroupSummary{
		Slug:                   msg.GetSlug(),
		Environment:            changelogEnvironmentLabel(msg.GetEnvironment()),
		OccurredAt:             timestampTime(msg.GetOccurredAt()),
		ReleaseVersion:         msg.GetReleaseVersion(),
		ReleaseTag:             cloneOptionalString(msg.ReleaseTag),
		PreviousReleaseVersion: cloneOptionalString(msg.PreviousReleaseVersion),
		Versions:               versions,
		Posts:                  posts,
	}
}

// ChangelogDeploymentGroupsPageFromProto decodes ListDeploymentGroupsResponse.
func ChangelogDeploymentGroupsPageFromProto(msg *changelogv1.ListDeploymentGroupsResponse) models.ChangelogDeploymentGroupsPage {
	groups := make([]models.ChangelogDeploymentGroupSummary, 0, len(msg.GetGroups()))
	for _, group := range msg.GetGroups() {
		if group == nil {
			continue
		}
		groups = append(groups, ChangelogDeploymentGroupSummaryFromProto(group))
	}
	return models.ChangelogDeploymentGroupsPage{Groups: groups, NextPageToken: msg.GetNextPageToken()}
}

// ChangelogDeploymentGroupFromProto decodes GetDeploymentGroupResponse. A
// success without a group, or a group without its summary, is a contract
// violation.
func ChangelogDeploymentGroupFromProto(msg *changelogv1.GetDeploymentGroupResponse) (models.ChangelogDeploymentGroup, error) {
	group := msg.GetGroup()
	if group == nil || group.GetSummary() == nil {
		return models.ChangelogDeploymentGroup{}, &sdkerrors.ResponseContractError{Operation: "GetDeploymentGroup", Msg: "missing deployment group"}
	}
	attention := make([]models.ChangelogDeploymentGroupAttention, 0, len(group.GetAttention()))
	for _, item := range group.GetAttention() {
		if item == nil {
			continue
		}
		attention = append(attention, models.ChangelogDeploymentGroupAttention{
			PostSlug:       item.GetPostSlug(),
			ReleaseVersion: item.GetReleaseVersion(),
			Change:         ChangelogChangeFromProto(item.GetChange()),
		})
	}
	highlights := make([]models.ChangelogDeploymentGroupHighlight, 0, len(group.GetHighlights()))
	for _, item := range group.GetHighlights() {
		if item == nil {
			continue
		}
		highlights = append(highlights, models.ChangelogDeploymentGroupHighlight{
			PostSlug:       item.GetPostSlug(),
			ReleaseVersion: item.GetReleaseVersion(),
			Text:           item.GetText(),
		})
	}
	return models.ChangelogDeploymentGroup{
		ChangelogDeploymentGroupSummary: ChangelogDeploymentGroupSummaryFromProto(group.GetSummary()),
		Attention:                       attention,
		Highlights:                      highlights,
	}, nil
}
