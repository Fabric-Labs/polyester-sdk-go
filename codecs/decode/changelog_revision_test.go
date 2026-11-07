package decode

import (
	"testing"
	"time"

	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A republished post keeps its slug and first publication time, and reports
// its current revision and when that revision was published.
func TestChangelogPostSummaryReadsRevisionAndUpdatedAt(t *testing.T) {
	published := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	summary := ChangelogPostSummaryFromProto(&changelogv1.PostSummary{
		Slug:        "2026-11-04",
		PublishedAt: timestamppb.New(published),
		UpdatedAt:   timestamppb.New(updated),
		Revision:    2,
	})
	if summary.Revision != 2 {
		t.Fatalf("revision = %d, want 2", summary.Revision)
	}
	if summary.UpdatedAt == nil || !summary.UpdatedAt.Equal(updated) || !summary.PublishedAt.Equal(published) {
		t.Fatalf("published_at = %v, updated_at = %v", summary.PublishedAt, summary.UpdatedAt)
	}

	post, err := ChangelogPostFromProto(&changelogv1.GetPostResponse{Post: &changelogv1.Post{Summary: &changelogv1.PostSummary{
		Slug: "2026-11-04", PublishedAt: timestamppb.New(published), UpdatedAt: timestamppb.New(published), Revision: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if post.Revision != 1 || post.UpdatedAt == nil || !post.UpdatedAt.Equal(published) {
		t.Fatalf("post = %+v", post.ChangelogPostSummary)
	}

	// A server before this field sends neither: revision 0, no update time.
	older := ChangelogPostSummaryFromProto(&changelogv1.PostSummary{Slug: "2026-10-01", PublishedAt: timestamppb.New(published)})
	if older.Revision != 0 || older.UpdatedAt != nil {
		t.Fatalf("older summary = %+v", older)
	}
}
