package polyester

import (
	"context"
	stderrors "errors"
	"reflect"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeDeploymentGroups serves deployment groups from memory, paged like
// fakeChangelog, on top of its posts.
type fakeDeploymentGroups struct {
	*fakeChangelog
	groups      []*changelogv1.DeploymentGroup
	groupCalls  []*changelogv1.ListDeploymentGroupsRequest
	emptyGroup  bool
	getSlugSeen []string
}

func (f *fakeDeploymentGroups) ListDeploymentGroups(_ context.Context, req *connect.Request[changelogv1.ListDeploymentGroupsRequest]) (*connect.Response[changelogv1.ListDeploymentGroupsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sawHeaders(req.Header())
	f.groupCalls = append(f.groupCalls, req.Msg)
	summaries := make([]*changelogv1.DeploymentGroupSummary, 0, len(f.groups))
	for _, group := range f.groups {
		summaries = append(summaries, group.GetSummary())
	}
	items, next, err := page(summaries, req.Msg.GetPageToken(), req.Msg.GetLimit(), 20)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&changelogv1.ListDeploymentGroupsResponse{Groups: items, NextPageToken: next}), nil
}

func (f *fakeDeploymentGroups) GetDeploymentGroup(_ context.Context, req *connect.Request[changelogv1.GetDeploymentGroupRequest]) (*connect.Response[changelogv1.GetDeploymentGroupResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sawHeaders(req.Header())
	f.getSlugSeen = append(f.getSlugSeen, req.Msg.GetSlug())
	if f.emptyGroup {
		return connect.NewResponse(&changelogv1.GetDeploymentGroupResponse{}), nil
	}
	for _, group := range f.groups {
		if group.GetSummary().GetSlug() == req.Msg.GetSlug() {
			return connect.NewResponse(&changelogv1.GetDeploymentGroupResponse{Group: group}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, stderrors.New("deployment group not found"))
}

var deploymentGroupTime = time.Date(2026, 11, 12, 9, 30, 0, 123_000_000, time.UTC)

// newFakeDeploymentGroups builds three testnet groups, newest first. Only the
// newest has a release tag and posts; the oldest has no previous release.
func newFakeDeploymentGroups() *fakeDeploymentGroups {
	f := &fakeDeploymentGroups{fakeChangelog: newFakeChangelog(1)}
	breaking := &changelogv1.Change{
		Id:       58,
		Title:    "Trigger activation price is fixed once tracking starts",
		Category: changelogv1.ChangeCategory_BREAKING_CHANGE,
		EnvironmentExceptions: []*changelogv1.ChangeException{
			{Environment: changelogv1.Environment_TESTNET, Availability: changelogv1.ChangeAvailability_AHEAD, LiveAt: timestamppb.New(deploymentGroupTime.Add(-time.Hour))},
			{Environment: changelogv1.Environment_MAINNET, Availability: changelogv1.ChangeAvailability_UNAVAILABLE},
		},
	}
	f.groups = []*changelogv1.DeploymentGroup{
		{
			Summary: &changelogv1.DeploymentGroupSummary{
				Slug:                   "testnet-v0.92.9",
				Environment:            changelogv1.Environment_TESTNET,
				OccurredAt:             timestamppb.New(deploymentGroupTime),
				ReleaseVersion:         "0.92.9",
				ReleaseTag:             ptr("v0.92.9"),
				PreviousReleaseVersion: ptr("0.92.7"),
				Versions:               []string{"0.92.8", "0.92.9"},
				Posts: []*changelogv1.DeploymentGroupPost{{
					Slug:           "v0.92.9",
					ReleaseVersion: "0.92.9",
					Title:          "Release 0.92.9",
					PublishedAt:    timestamppb.New(deploymentGroupTime.Add(-24 * time.Hour)),
					Revision:       2,
					UpdatedAt:      timestamppb.New(deploymentGroupTime.Add(-2 * time.Hour)),
				}},
			},
			Attention:  []*changelogv1.DeploymentGroupAttention{{PostSlug: "v0.92.9", ReleaseVersion: "0.92.9", Change: breaking}},
			Highlights: []*changelogv1.DeploymentGroupHighlight{{PostSlug: "v0.92.9", ReleaseVersion: "0.92.9", Text: "**Faster** order entry"}},
		},
		{Summary: &changelogv1.DeploymentGroupSummary{Slug: "testnet-v0.92.7", Environment: changelogv1.Environment_TESTNET, ReleaseVersion: "0.92.7", PreviousReleaseVersion: ptr("0.92.5"), Versions: []string{"0.92.6", "0.92.7"}}},
		{Summary: &changelogv1.DeploymentGroupSummary{Slug: "testnet-v0.92.5", Environment: changelogv1.Environment(9), ReleaseVersion: "0.92.5", Versions: []string{"0.92.5"}}},
	}
	return f
}

func newDeploymentGroupsClient(t *testing.T, fake *fakeDeploymentGroups) *Client {
	t.Helper()
	return newPublicChangelogClient(t, fake)
}

func TestChangelogDeploymentGroupsIterateEveryPageWithoutCredentials(t *testing.T) {
	fake := newFakeDeploymentGroups()
	client := newDeploymentGroupsClient(t, fake)

	var groups []models.ChangelogDeploymentGroupSummary
	for group, err := range client.Changelog.DeploymentGroups(context.Background(), models.ListChangelogDeploymentGroupsOptions{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		groups = append(groups, group)
	}
	slugs := make([]string, 0, len(groups))
	for _, group := range groups {
		slugs = append(slugs, group.Slug)
	}
	if want := []string{"testnet-v0.92.9", "testnet-v0.92.7", "testnet-v0.92.5"}; !reflect.DeepEqual(slugs, want) {
		t.Fatalf("slugs = %v, want %v", slugs, want)
	}
	if len(fake.groupCalls) != 2 || fake.groupCalls[0].GetLimit() != 2 || fake.groupCalls[1].GetPageToken() != "2" {
		t.Fatalf("calls = %v", fake.groupCalls)
	}
	if fake.authHeaderSet {
		t.Fatal("changelog requests must not carry API-key auth headers")
	}

	newest := groups[0]
	occurred := deploymentGroupTime
	published := deploymentGroupTime.Add(-24 * time.Hour)
	updated := deploymentGroupTime.Add(-2 * time.Hour)
	want := models.ChangelogDeploymentGroupSummary{
		Slug:                   "testnet-v0.92.9",
		Environment:            models.ChangelogEnvironmentTestnet,
		OccurredAt:             &occurred,
		ReleaseVersion:         "0.92.9",
		ReleaseTag:             ptr("v0.92.9"),
		PreviousReleaseVersion: ptr("0.92.7"),
		Versions:               []string{"0.92.8", "0.92.9"},
		Posts: []models.ChangelogDeploymentGroupPost{{
			Slug: "v0.92.9", ReleaseVersion: "0.92.9", Title: "Release 0.92.9",
			PublishedAt: &published, Revision: 2, UpdatedAt: &updated,
		}},
	}
	if !reflect.DeepEqual(newest, want) {
		t.Fatalf("newest group\n got %+v\nwant %+v", newest, want)
	}
	// Omitted optional fields and timestamps stay nil, not empty or zero.
	middle := groups[1]
	if middle.ReleaseTag != nil || middle.OccurredAt != nil || middle.PreviousReleaseVersion == nil || len(middle.Posts) != 0 {
		t.Fatalf("middle group %+v", middle)
	}
	oldest := groups[2]
	if oldest.PreviousReleaseVersion != nil || oldest.Environment != "unknown(9)" {
		t.Fatalf("oldest group %+v", oldest)
	}
}

func TestChangelogListDeploymentGroupsSendsPageToken(t *testing.T) {
	fake := newFakeDeploymentGroups()
	client := newDeploymentGroupsClient(t, fake)

	page, err := client.Changelog.ListDeploymentGroups(context.Background(), models.ListChangelogDeploymentGroupsOptions{Limit: 1, PageToken: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Slug != "testnet-v0.92.7" || page.NextPageToken != "2" {
		t.Fatalf("page = %+v", page)
	}
	if _, err := client.Changelog.ListDeploymentGroups(context.Background(), models.ListChangelogDeploymentGroupsOptions{Limit: -1}); err == nil {
		t.Fatal("a negative limit was sent")
	}
}

func TestChangelogGetDeploymentGroupDecodesAttentionAndHighlights(t *testing.T) {
	fake := newFakeDeploymentGroups()
	client := newDeploymentGroupsClient(t, fake)

	group, err := client.Changelog.GetDeploymentGroup(context.Background(), "testnet-v0.92.9")
	if err != nil {
		t.Fatal(err)
	}
	if group.Slug != "testnet-v0.92.9" || group.ReleaseVersion != "0.92.9" || len(group.Posts) != 1 {
		t.Fatalf("summary = %+v", group.ChangelogDeploymentGroupSummary)
	}
	if len(group.Attention) != 1 {
		t.Fatalf("attention = %+v", group.Attention)
	}
	attention := group.Attention[0]
	if attention.PostSlug != "v0.92.9" || attention.ReleaseVersion != "0.92.9" || attention.Change.Category != models.ChangelogCategoryBreakingChange || attention.Change.ID == "" {
		t.Fatalf("attention = %+v", attention)
	}
	ahead := deploymentGroupTime.Add(-time.Hour)
	wantExceptions := []models.ChangelogChangeException{
		{Environment: models.ChangelogEnvironmentTestnet, Availability: models.ChangelogAvailabilityAhead, LiveAt: &ahead},
		{Environment: models.ChangelogEnvironmentMainnet, Availability: models.ChangelogAvailabilityUnavailable},
	}
	if !reflect.DeepEqual(attention.Change.EnvironmentExceptions, wantExceptions) {
		t.Fatalf("exceptions = %+v", attention.Change.EnvironmentExceptions)
	}
	if want := []models.ChangelogDeploymentGroupHighlight{{PostSlug: "v0.92.9", ReleaseVersion: "0.92.9", Text: "**Faster** order entry"}}; !reflect.DeepEqual(group.Highlights, want) {
		t.Fatalf("highlights = %+v", group.Highlights)
	}
	if fake.authHeaderSet || !reflect.DeepEqual(fake.getSlugSeen, []string{"testnet-v0.92.9"}) {
		t.Fatalf("auth=%v slugs=%v", fake.authHeaderSet, fake.getSlugSeen)
	}
}

func TestChangelogGetDeploymentGroupUnknownSlugIsNotFound(t *testing.T) {
	client := newDeploymentGroupsClient(t, newFakeDeploymentGroups())
	_, err := client.Changelog.GetDeploymentGroup(context.Background(), "testnet-v9.9.9")
	var apiErr *sdkerrors.APIError
	if !stderrors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("err = %v", err)
	}
}

func TestChangelogGetDeploymentGroupWithoutAGroupIsAContractViolation(t *testing.T) {
	fake := newFakeDeploymentGroups()
	fake.emptyGroup = true
	client := newDeploymentGroupsClient(t, fake)
	_, err := client.Changelog.GetDeploymentGroup(context.Background(), "testnet-v0.92.9")
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) {
		t.Fatalf("err = %v", err)
	}
}

// Release posts carry their release, its status in each environment and the
// deployment group that brought it; other posts carry none of them.
func TestChangelogPostSummariesCarryTheirRelease(t *testing.T) {
	fake := newFakeChangelog(2)
	release := fake.posts[0].GetSummary()
	release.ReleaseVersion = ptr("0.92.9")
	release.DeploymentGroupSlug = ptr("testnet-v0.92.9")
	release.Environments = []*changelogv1.EnvironmentStatus{
		{Environment: changelogv1.Environment_DEVNET, LiveAt: timestamppb.New(deploymentGroupTime.Add(-48 * time.Hour))},
		{Environment: changelogv1.Environment_TESTNET, LiveAt: timestamppb.New(deploymentGroupTime)},
		{Environment: changelogv1.Environment_MAINNET},
	}
	client := newPublicChangelogClient(t, fake)

	page, err := client.Changelog.ListPosts(context.Background(), models.ListChangelogPostsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	devnet, testnet := deploymentGroupTime.Add(-48*time.Hour), deploymentGroupTime
	got := page.Posts[0]
	if got.ReleaseVersion == nil || *got.ReleaseVersion != "0.92.9" || got.DeploymentGroupSlug == nil || *got.DeploymentGroupSlug != "testnet-v0.92.9" {
		t.Fatalf("release post = %+v", got)
	}
	wantEnvironments := []models.ChangelogEnvironmentStatus{
		{Environment: models.ChangelogEnvironmentDevnet, LiveAt: &devnet},
		{Environment: models.ChangelogEnvironmentTestnet, LiveAt: &testnet},
		{Environment: models.ChangelogEnvironmentMainnet},
	}
	if !reflect.DeepEqual(got.Environments, wantEnvironments) {
		t.Fatalf("environments = %+v", got.Environments)
	}
	other := page.Posts[1]
	if other.ReleaseVersion != nil || other.DeploymentGroupSlug != nil || other.Environments != nil {
		t.Fatalf("non-release post = %+v", other)
	}

	post, err := client.Changelog.GetPost(context.Background(), got.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(post.ChangelogPostSummary, got) {
		t.Fatalf("GetPost summary = %+v, want %+v", post.ChangelogPostSummary, got)
	}
}
