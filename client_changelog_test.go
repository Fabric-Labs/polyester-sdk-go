package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1/changelogv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeChangelog serves posts and changes from memory and pages them with the
// item offset as the page token.
type fakeChangelog struct {
	changelogv1connect.UnimplementedChangelogServiceHandler

	posts   []*changelogv1.Post
	entries []*changelogv1.ChangeEntry

	mu            sync.Mutex
	postsCalls    []*changelogv1.ListPostsRequest
	changesCalls  []*changelogv1.ListChangesRequest
	authHeaderSet bool
	repeatToken   bool
	// cycleTokens, when set, replaces every ListPosts next_page_token in
	// order: a server whose tokens cycle back to an earlier page.
	cycleTokens []string
}

func (f *fakeChangelog) sawHeaders(h http.Header) {
	if h.Get("Authorization") != "" || h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
		f.authHeaderSet = true
	}
}

func page[T any](items []T, token string, limit uint32, fallback int) ([]T, string, error) {
	start := 0
	if token != "" {
		n, err := strconv.Atoi(token)
		if err != nil || n < 0 || n > len(items) {
			return nil, "", connect.NewError(connect.CodeInvalidArgument, stderrors.New("invalid page_token"))
		}
		start = n
	}
	size := int(limit)
	if size == 0 {
		size = fallback
	}
	end := min(start+size, len(items))
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return items[start:end], next, nil
}

func (f *fakeChangelog) ListPosts(_ context.Context, req *connect.Request[changelogv1.ListPostsRequest]) (*connect.Response[changelogv1.ListPostsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sawHeaders(req.Header())
	f.postsCalls = append(f.postsCalls, req.Msg)
	summaries := make([]*changelogv1.PostSummary, 0, len(f.posts))
	for _, post := range f.posts {
		summaries = append(summaries, post.GetSummary())
	}
	if f.cycleTokens != nil {
		next := f.cycleTokens[(len(f.postsCalls)-1)%len(f.cycleTokens)]
		return connect.NewResponse(&changelogv1.ListPostsResponse{Posts: summaries[:1], NextPageToken: next}), nil
	}
	items, next, err := page(summaries, req.Msg.GetPageToken(), req.Msg.GetLimit(), 20)
	if err != nil {
		return nil, err
	}
	if f.repeatToken && req.Msg.GetPageToken() != "" {
		next = req.Msg.GetPageToken()
	}
	return connect.NewResponse(&changelogv1.ListPostsResponse{Posts: items, NextPageToken: next}), nil
}

func (f *fakeChangelog) GetPost(_ context.Context, req *connect.Request[changelogv1.GetPostRequest]) (*connect.Response[changelogv1.GetPostResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sawHeaders(req.Header())
	for _, post := range f.posts {
		if post.GetSummary().GetSlug() == req.Msg.GetSlug() {
			return connect.NewResponse(&changelogv1.GetPostResponse{Post: post}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, stderrors.New("post not found"))
}

func (f *fakeChangelog) ListChanges(_ context.Context, req *connect.Request[changelogv1.ListChangesRequest]) (*connect.Response[changelogv1.ListChangesResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sawHeaders(req.Header())
	f.changesCalls = append(f.changesCalls, req.Msg)
	if req.Msg.StartTime != nil && req.Msg.EndTime != nil && !req.Msg.GetEndTime().AsTime().After(req.Msg.GetStartTime().AsTime()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, stderrors.New("end_time must be after start_time"))
	}
	matched := make([]*changelogv1.ChangeEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		change := entry.GetChange()
		if len(req.Msg.GetCategories()) > 0 && !containsEnum(req.Msg.GetCategories(), change.GetCategory()) {
			continue
		}
		if req.Msg.ActionRequired != nil && change.GetActionRequired() != req.Msg.GetActionRequired() {
			continue
		}
		if req.Msg.StartTime != nil && entry.GetPublishedAt().AsTime().Before(req.Msg.GetStartTime().AsTime()) {
			continue
		}
		matched = append(matched, entry)
	}
	items, next, err := page(matched, req.Msg.GetPageToken(), req.Msg.GetLimit(), 100)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&changelogv1.ListChangesResponse{Changes: items, NextPageToken: next}), nil
}

func containsEnum[E comparable](values []E, value E) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func ptr[T any](v T) *T { return &v }

// newFakeChangelog builds posts newest first: post i is published i days
// before 2026-10-31 and holds two changes. Change ids are 1000+n.
func newFakeChangelog(postCount int) *fakeChangelog {
	f := &fakeChangelog{}
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	categories := []changelogv1.ChangeCategory{
		changelogv1.ChangeCategory_BREAKING_CHANGE,
		changelogv1.ChangeCategory_NEW_FEATURE,
		changelogv1.ChangeCategory_IMPROVEMENT,
		changelogv1.ChangeCategory_BUG_FIX,
	}
	n := 0
	for i := range postCount {
		published := base.AddDate(0, 0, -i)
		slug := published.Format("2006-01-02")
		changes := make([]*changelogv1.Change, 0, 2)
		for range 2 {
			changes = append(changes, &changelogv1.Change{
				Id:             uint64(1000 + n),
				Title:          "change " + strconv.Itoa(n),
				Summary:        "summary " + strconv.Itoa(n),
				Category:       categories[n%len(categories)],
				ActionRequired: n%3 == 0,
				Audiences:      []changelogv1.Audience{changelogv1.Audience_DEVELOPERS},
				Surfaces:       []changelogv1.Surface{changelogv1.Surface_API},
			})
			n++
		}
		post := &changelogv1.Post{
			Summary: &changelogv1.PostSummary{
				Slug:           slug,
				Title:          "post " + slug,
				PublishedAt:    timestamppb.New(published),
				CoverageCutoff: timestamppb.New(published.Add(-time.Hour)),
				ChangeCount:    2,
			},
			Sections: []*changelogv1.Section{{Heading: "API", Summary: ptr("API changes"), Changes: changes}},
		}
		f.posts = append(f.posts, post)
		for _, change := range changes {
			f.entries = append(f.entries, &changelogv1.ChangeEntry{
				Change:         change,
				PostSlug:       slug,
				PublishedAt:    timestamppb.New(published),
				SectionHeading: "API",
			})
		}
	}
	return f
}

// newPublicChangelogClient serves fake through Connect and returns a client
// built without API-key credentials.
func newPublicChangelogClient(t *testing.T, fake *fakeChangelog) *Client {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := changelogv1connect.NewChangelogServiceHandler(fake)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client, err := New(Config{APIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestChangelogPostsIteratesEveryPageWithoutCredentials(t *testing.T) {
	fake := newFakeChangelog(5)
	client := newPublicChangelogClient(t, fake)

	var slugs []string
	for post, err := range client.Changelog.Posts(context.Background(), models.ListChangelogPostsOptions{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		slugs = append(slugs, post.Slug)
	}
	want := []string{"2026-10-31", "2026-10-30", "2026-10-29", "2026-10-28", "2026-10-27"}
	if !reflect.DeepEqual(slugs, want) {
		t.Fatalf("slugs = %v, want %v", slugs, want)
	}
	if len(fake.postsCalls) != 3 {
		t.Fatalf("ListPosts calls = %d, want 3", len(fake.postsCalls))
	}
	for i, token := range []string{"", "2", "4"} {
		if got := fake.postsCalls[i].GetPageToken(); got != token {
			t.Fatalf("call %d page_token = %q, want %q", i, got, token)
		}
		if got := fake.postsCalls[i].GetLimit(); got != 2 {
			t.Fatalf("call %d limit = %d, want 2", i, got)
		}
	}
	if fake.authHeaderSet {
		t.Fatal("changelog requests must not carry API-key auth headers")
	}
}

func TestChangelogPostsStopsFetchingWhenTheCallerBreaks(t *testing.T) {
	fake := newFakeChangelog(5)
	client := newPublicChangelogClient(t, fake)

	for post, err := range client.Changelog.Posts(context.Background(), models.ListChangelogPostsOptions{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		if post.Slug == "2026-10-31" {
			break
		}
	}
	if len(fake.postsCalls) != 1 {
		t.Fatalf("ListPosts calls = %d, want 1", len(fake.postsCalls))
	}
}

func TestChangelogPostsRejectsARepeatedPageToken(t *testing.T) {
	fake := newFakeChangelog(5)
	fake.repeatToken = true
	client := newPublicChangelogClient(t, fake)

	var got error
	count := 0
	for _, err := range client.Changelog.Posts(context.Background(), models.ListChangelogPostsOptions{Limit: 2}) {
		if err != nil {
			got = err
			break
		}
		count++
	}
	var contractErr *sdkerrors.ResponseContractError
	if !stderrors.As(got, &contractErr) {
		t.Fatalf("err = %v, want ResponseContractError", got)
	}
	if count != 4 {
		t.Fatalf("yielded %d posts before the error, want 4", count)
	}
}

func TestChangelogPostsRejectsATokenCycle(t *testing.T) {
	for name, tc := range map[string]struct {
		start  string
		cycle  []string
		calls  []string
		yields int
	}{
		// "" -> A -> B -> A: A recurs after two pages.
		"two tokens": {cycle: []string{"A", "B", "A"}, calls: []string{"", "A", "B"}, yields: 3},
		// "" -> A -> B -> C -> B.
		"three tokens": {cycle: []string{"A", "B", "C", "B"}, calls: []string{"", "A", "B", "C"}, yields: 4},
		// The caller's starting token counts: S -> A -> S.
		"starting token": {start: "S", cycle: []string{"A", "S"}, calls: []string{"S", "A"}, yields: 2},
	} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeChangelog(5)
			fake.cycleTokens = tc.cycle
			client := newPublicChangelogClient(t, fake)

			var got error
			count := 0
			for _, err := range client.Changelog.Posts(context.Background(), models.ListChangelogPostsOptions{PageToken: tc.start}) {
				if err != nil {
					got = err
					break
				}
				count++
			}
			var contractErr *sdkerrors.ResponseContractError
			if !stderrors.As(got, &contractErr) {
				t.Fatalf("err = %v, want ResponseContractError", got)
			}
			if count != tc.yields {
				t.Fatalf("yielded %d posts, want %d", count, tc.yields)
			}
			calls := make([]string, 0, len(fake.postsCalls))
			for _, req := range fake.postsCalls {
				calls = append(calls, req.GetPageToken())
			}
			if !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("page tokens sent = %q, want %q", calls, tc.calls)
			}
		})
	}
}

func TestChangelogGetPostDecodesSectionsAndBase58IDs(t *testing.T) {
	fake := newFakeChangelog(2)
	client := newPublicChangelogClient(t, fake)

	post, err := client.Changelog.GetPost(context.Background(), "2026-10-30")
	if err != nil {
		t.Fatal(err)
	}
	if post.Slug != "2026-10-30" || post.ChangeCount != 2 || post.PublishedAt == nil {
		t.Fatalf("post summary = %+v", post.ChangelogPostSummary)
	}
	if len(post.Sections) != 1 || post.Sections[0].Heading != "API" || *post.Sections[0].Summary != "API changes" {
		t.Fatalf("sections = %+v", post.Sections)
	}
	change := post.Sections[0].Changes[0]
	// 1002 in base58 is "JH"; the decimal form must never leak.
	if change.ID != "JH" {
		t.Fatalf("change id = %q, want base58 JH", change.ID)
	}
	if change.Category != models.ChangelogCategoryImprovement {
		t.Fatalf("category = %q", change.Category)
	}
	if !reflect.DeepEqual(change.Audiences, []string{models.ChangelogAudienceDevelopers}) ||
		!reflect.DeepEqual(change.Surfaces, []string{models.ChangelogSurfaceAPI}) {
		t.Fatalf("audiences=%v surfaces=%v", change.Audiences, change.Surfaces)
	}
}

func TestChangelogGetPostUnknownSlugIsNotFound(t *testing.T) {
	client := newPublicChangelogClient(t, newFakeChangelog(1))

	_, err := client.Changelog.GetPost(context.Background(), "2001-01-01")
	var apiErr *sdkerrors.APIError
	if !stderrors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("err = %#v, want APIError not_found", err)
	}
	if !stderrors.Is(err, sdkerrors.ErrPolyester) {
		t.Fatal("not_found must match ErrPolyester")
	}
}

func TestChangelogToleratesEnumValuesNewerThanTheSDK(t *testing.T) {
	fake := newFakeChangelog(1)
	change := fake.posts[0].GetSections()[0].GetChanges()[0]
	change.Category = changelogv1.ChangeCategory(99)
	change.Audiences = []changelogv1.Audience{changelogv1.Audience_TRADERS, changelogv1.Audience(42)}
	change.Surfaces = []changelogv1.Surface{changelogv1.Surface(77), changelogv1.Surface_TRADING}
	client := newPublicChangelogClient(t, fake)

	post, err := client.Changelog.GetPost(context.Background(), "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	got := post.Sections[0].Changes[0]
	if got.Category != "unknown(99)" {
		t.Fatalf("category = %q", got.Category)
	}
	if !reflect.DeepEqual(got.Audiences, []string{"traders", "unknown(42)"}) {
		t.Fatalf("audiences = %v", got.Audiences)
	}
	if !reflect.DeepEqual(got.Surfaces, []string{"unknown(77)", "trading"}) {
		t.Fatalf("surfaces = %v", got.Surfaces)
	}
}

func TestChangelogBreakingChangesSinceFiltersAndPagesEverything(t *testing.T) {
	fake := newFakeChangelog(600)
	client := newPublicChangelogClient(t, fake)
	since := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC).AddDate(0, 0, -599)

	changes, err := client.Changelog.BreakingChangesSince(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	// 1200 changes cycle through four categories, so 300 are breaking.
	if len(changes) != 300 {
		t.Fatalf("breaking changes = %d, want 300", len(changes))
	}
	for _, entry := range changes {
		if entry.Change.Category != models.ChangelogCategoryBreakingChange {
			t.Fatalf("non-breaking change %+v", entry.Change)
		}
	}
	req := fake.changesCalls[0]
	if !reflect.DeepEqual(req.GetCategories(), []changelogv1.ChangeCategory{changelogv1.ChangeCategory_BREAKING_CHANGE}) ||
		!req.GetStartTime().AsTime().Equal(since) || req.ActionRequired != nil || req.GetLimit() != 500 {
		t.Fatalf("request = %v", req)
	}
	if len(fake.changesCalls) != 1 {
		t.Fatalf("ListChanges calls = %d, want 1 page of 300", len(fake.changesCalls))
	}
}

func TestChangelogActionRequiredSinceFollowsPageTokens(t *testing.T) {
	fake := newFakeChangelog(1000)
	client := newPublicChangelogClient(t, fake)
	since := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC).AddDate(0, 0, -999)

	changes, err := client.Changelog.ActionRequiredSince(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	// Every third of 2000 changes needs action: 667 of them, over two pages.
	if len(changes) != 667 {
		t.Fatalf("action-required changes = %d, want 667", len(changes))
	}
	if len(fake.changesCalls) != 2 || fake.changesCalls[1].GetPageToken() != "500" {
		t.Fatalf("ListChanges calls = %v", fake.changesCalls)
	}
	for _, req := range fake.changesCalls {
		if req.ActionRequired == nil || !req.GetActionRequired() || len(req.GetCategories()) != 0 {
			t.Fatalf("request = %v", req)
		}
	}
	seen := map[string]bool{}
	for _, entry := range changes {
		if !entry.Change.ActionRequired || seen[entry.Change.ID] {
			t.Fatalf("unexpected entry %+v", entry.Change)
		}
		seen[entry.Change.ID] = true
	}
}

func TestChangelogListChangesSendsFiltersAndMapsInvalidArgument(t *testing.T) {
	fake := newFakeChangelog(1)
	client := newPublicChangelogClient(t, fake)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	_, err := client.Changelog.ListChanges(context.Background(), models.ListChangelogChangesOptions{
		Categories: []string{"new_feature", "BUG_FIX"},
		Surfaces:   []string{models.ChangelogSurfaceTrading},
		Audiences:  []string{models.ChangelogAudienceMarketMakers},
		StartTime:  &start,
		EndTime:    &start,
		PostSlug:   "2026-10-31",
		Sort:       "asc",
		Limit:      10,
	})
	var validationErr *sdkerrors.ValidationError
	if !stderrors.As(err, &validationErr) || validationErr.Code != "invalid_argument" {
		t.Fatalf("err = %#v, want server invalid_argument", err)
	}
	req := fake.changesCalls[0]
	if !reflect.DeepEqual(req.GetCategories(), []changelogv1.ChangeCategory{changelogv1.ChangeCategory_NEW_FEATURE, changelogv1.ChangeCategory_BUG_FIX}) ||
		!reflect.DeepEqual(req.GetSurfaces(), []changelogv1.Surface{changelogv1.Surface_TRADING}) ||
		!reflect.DeepEqual(req.GetAudiences(), []changelogv1.Audience{changelogv1.Audience_MARKET_MAKERS}) ||
		req.GetPostSlug() != "2026-10-31" || req.GetSort() != changelogv1.SortDirection_SORT_ASC || req.GetLimit() != 10 {
		t.Fatalf("request = %v", req)
	}
}

func TestChangelogListChangesRejectsLabelsItCannotSend(t *testing.T) {
	fake := newFakeChangelog(1)
	client := newPublicChangelogClient(t, fake)

	for name, opts := range map[string]models.ListChangelogChangesOptions{
		"category":    {Categories: []string{"unknown(9)"}},
		"unspecified": {Surfaces: []string{"surface_unspecified"}},
		"audience":    {Audiences: []string{"whales"}},
		"sort":        {Sort: "newest"},
		"limit":       {Limit: -1},
	} {
		_, err := client.Changelog.ListChanges(context.Background(), opts)
		var validationErr *sdkerrors.ValidationError
		if !stderrors.As(err, &validationErr) {
			t.Fatalf("%s: err = %v, want ValidationError", name, err)
		}
	}
	if len(fake.changesCalls) != 0 {
		t.Fatalf("invalid options reached the server %d times", len(fake.changesCalls))
	}
}
