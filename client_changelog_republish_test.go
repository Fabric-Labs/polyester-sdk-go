package polyester

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1/changelogv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// republishingChangelog answers ListChanges in two pages. Between them the
// post the first page's token points into is republished, so the second page
// restarts at that post's first change and repeats change 1002 with its new
// wording, as the server does.
type republishingChangelog struct {
	changelogv1connect.UnimplementedChangelogServiceHandler
	calls int
}

func republishedEntry(id uint64, title string, category changelogv1.ChangeCategory, slug string, published time.Time) *changelogv1.ChangeEntry {
	return &changelogv1.ChangeEntry{
		Change:      &changelogv1.Change{Id: id, Title: title, Category: category, ActionRequired: true},
		PostSlug:    slug,
		PublishedAt: timestamppb.New(published),
	}
}

func (f *republishingChangelog) ListChanges(_ context.Context, req *connect.Request[changelogv1.ListChangesRequest]) (*connect.Response[changelogv1.ListChangesResponse], error) {
	f.calls++
	newer := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	older := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	breaking := changelogv1.ChangeCategory_BREAKING_CHANGE
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&changelogv1.ListChangesResponse{
			Changes: []*changelogv1.ChangeEntry{
				republishedEntry(1001, "newer post change", breaking, "2026-11-06", newer),
				republishedEntry(1002, "first wording", breaking, "2026-11-04", older),
			},
			NextPageToken: "t1",
		}), nil
	}
	return connect.NewResponse(&changelogv1.ListChangesResponse{
		Changes: []*changelogv1.ChangeEntry{
			republishedEntry(1002, "corrected wording", breaking, "2026-11-04", older),
			republishedEntry(1003, "third", breaking, "2026-11-04", older),
		},
	}), nil
}

func newRepublishingChangelogClient(t *testing.T) (*Client, *republishingChangelog) {
	t.Helper()
	fake := &republishingChangelog{}
	mux := http.NewServeMux()
	path, handler := changelogv1connect.NewChangelogServiceHandler(fake)
	mux.Handle(path, handler)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	client, err := New(Config{APIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, fake
}

func changeIDsAndTitles(entries []models.ChangelogChangeEntry) ([]string, []string) {
	ids := make([]string, 0, len(entries))
	titles := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Change.ID)
		titles = append(titles, entry.Change.Title)
	}
	return ids, titles
}

func TestChangelogChangesYieldsARepublishedChangeOnce(t *testing.T) {
	client, fake := newRepublishingChangelogClient(t)
	var entries []models.ChangelogChangeEntry
	for entry, err := range client.Changelog.Changes(context.Background(), models.ListChangelogChangesOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	ids, _ := changeIDsAndTitles(entries)
	if fake.calls != 2 || len(ids) != 3 || ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] {
		t.Fatalf("calls = %d, change ids = %v, want three distinct", fake.calls, ids)
	}
}

func TestChangelogSinceHelpersKeepOneCurrentCopyOfARepublishedChange(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for name, call := range map[string]func(*Client) ([]models.ChangelogChangeEntry, error){
		"BreakingChangesSince": func(c *Client) ([]models.ChangelogChangeEntry, error) {
			return c.Changelog.BreakingChangesSince(context.Background(), since)
		},
		"ActionRequiredSince": func(c *Client) ([]models.ChangelogChangeEntry, error) {
			return c.Changelog.ActionRequiredSince(context.Background(), since)
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newRepublishingChangelogClient(t)
			entries, err := call(client)
			if err != nil {
				t.Fatal(err)
			}
			ids, titles := changeIDsAndTitles(entries)
			if len(ids) != 3 {
				t.Fatalf("change ids = %v, want three distinct", ids)
			}
			// The repeat keeps its place and carries the current revision's text.
			if titles[1] != "corrected wording" {
				t.Fatalf("titles = %v, want the republished wording second", titles)
			}
		})
	}
}
