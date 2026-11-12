package services

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1/changelogv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// changelogSincePageSize is the page size of the *Since helpers: the server
// maximum, so a long history costs as few rate-limited requests as possible.
const changelogSincePageSize = 500

// ChangelogService reads the public product changelog of the environment the
// client points at. It needs no credentials.
type ChangelogService struct {
	transport *transport.Factory
}

func NewChangelogService(factory *transport.Factory) *ChangelogService {
	return &ChangelogService{transport: factory}
}

func (s *ChangelogService) client() changelogv1connect.ChangelogServiceClient {
	return changelogv1connect.NewChangelogServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

// ListPosts returns one page of post summaries, newest first.
func (s *ChangelogService) ListPosts(ctx context.Context, opts models.ListChangelogPostsOptions) (models.ChangelogPostsPage, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return models.ChangelogPostsPage{}, err
	}
	req := &changelogv1.ListPostsRequest{Limit: limit, PageToken: opts.PageToken}
	return UnaryPublic(ctx, s.transport, s.client().ListPosts, req, decode.ChangelogPostsPageFromProto)
}

// GetPost returns one full post by slug. An unknown slug fails with an
// *errors.APIError whose Code is "not_found".
func (s *ChangelogService) GetPost(ctx context.Context, slug string) (models.ChangelogPost, error) {
	req := &changelogv1.GetPostRequest{Slug: slug}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetPost, req, decode.ChangelogPostFromProto)
}

// ListChanges returns one page of changes across posts.
func (s *ChangelogService) ListChanges(ctx context.Context, opts models.ListChangelogChangesOptions) (models.ChangelogChangesPage, error) {
	req, err := listChangesRequest(opts)
	if err != nil {
		return models.ChangelogChangesPage{}, err
	}
	return UnaryPublic(ctx, s.transport, s.client().ListChanges, req, decode.ChangelogChangesPageFromProto)
}

// Posts iterates over post summaries from opts.PageToken (or the newest post)
// to the oldest, fetching pages as needed. Iteration stops at the first error.
func (s *ChangelogService) Posts(ctx context.Context, opts models.ListChangelogPostsOptions) iter.Seq2[models.ChangelogPostSummary, error] {
	return paginate(opts.PageToken, func(token string) ([]models.ChangelogPostSummary, string, error) {
		opts.PageToken = token
		page, err := s.ListPosts(ctx, opts)
		return page.Posts, page.NextPageToken, err
	})
}

// Changes iterates over every change matching opts, fetching pages as
// needed. Iteration stops at the first error.
//
// When a post is republished during the walk, the next page restarts at that
// post's first change, so changes already yielded can come again. Each change
// ID is yielded once per walk, as first received; read the post with GetPost
// for its current wording.
func (s *ChangelogService) Changes(ctx context.Context, opts models.ListChangelogChangesOptions) iter.Seq2[models.ChangelogChangeEntry, error] {
	return func(yield func(models.ChangelogChangeEntry, error) bool) {
		seen := map[string]struct{}{}
		for entry, err := range s.changeWalk(ctx, opts) {
			if err == nil {
				if _, repeat := seen[entry.Change.ID]; repeat {
					continue
				}
				seen[entry.Change.ID] = struct{}{}
			}
			if !yield(entry, err) {
				return
			}
		}
	}
}

// changeWalk yields every ListChanges entry of one walk, repeats included.
func (s *ChangelogService) changeWalk(ctx context.Context, opts models.ListChangelogChangesOptions) iter.Seq2[models.ChangelogChangeEntry, error] {
	return paginate(opts.PageToken, func(token string) ([]models.ChangelogChangeEntry, string, error) {
		opts.PageToken = token
		page, err := s.ListChanges(ctx, opts)
		return page.Changes, page.NextPageToken, err
	})
}

// BreakingChangesSince returns every breaking change published at or after
// since, newest post first.
//
// It filters on the post's first publication time, which a republication does
// not move, so an edit to an older post is not reported: compare the
// Revision of ListPosts summaries to find edited posts. A change met again
// because its post was republished during the call is returned once, at its
// first position, with the wording last received.
func (s *ChangelogService) BreakingChangesSince(ctx context.Context, since time.Time) ([]models.ChangelogChangeEntry, error) {
	return collectChanges(s.changeWalk(ctx, models.ListChangelogChangesOptions{
		Categories: []string{models.ChangelogCategoryBreakingChange},
		StartTime:  &since,
		Limit:      changelogSincePageSize,
	}))
}

// ActionRequiredSince returns every change that requires integrator action
// published at or after since, newest post first. Republished posts are
// handled as in BreakingChangesSince.
func (s *ChangelogService) ActionRequiredSince(ctx context.Context, since time.Time) ([]models.ChangelogChangeEntry, error) {
	actionRequired := true
	return collectChanges(s.changeWalk(ctx, models.ListChangelogChangesOptions{
		ActionRequired: &actionRequired,
		StartTime:      &since,
		Limit:          changelogSincePageSize,
	}))
}

// collectChanges collects one walk, keeping one entry per change ID: a repeat
// replaces the earlier entry in place, so the latest revision's wording wins.
func collectChanges(seq iter.Seq2[models.ChangelogChangeEntry, error]) ([]models.ChangelogChangeEntry, error) {
	out := []models.ChangelogChangeEntry{}
	index := map[string]int{}
	for entry, err := range seq {
		if err != nil {
			return nil, err
		}
		if i, repeat := index[entry.Change.ID]; repeat {
			out[i] = entry
			continue
		}
		index[entry.Change.ID] = len(out)
		out = append(out, entry)
	}
	return out, nil
}

// paginate yields every item of fetch's pages, starting from token, until a
// page has no next token. A next token already requested in this iteration
// would cycle forever, so it ends iteration with a contract error.
func paginate[T any](token string, fetch func(token string) ([]T, string, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		token := token
		requested := map[string]struct{}{}
		for {
			if token != "" {
				requested[token] = struct{}{}
			}
			items, next, err := fetch(token)
			if err != nil {
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if next == "" {
				return
			}
			if _, seen := requested[next]; seen {
				yield(zero, &sdkerrors.ResponseContractError{Msg: "next_page_token repeats an earlier page_token"})
				return
			}
			token = next
		}
	}
}

func listChangesRequest(opts models.ListChangelogChangesOptions) (*changelogv1.ListChangesRequest, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return nil, err
	}
	categories, err := changelogEnums[changelogv1.ChangeCategory](opts.Categories, changelogv1.ChangeCategory_value, "categories")
	if err != nil {
		return nil, err
	}
	if len(opts.Surfaces) > maxChangelogSurfaces {
		return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("surfaces accepts at most %d values, got %d", maxChangelogSurfaces, len(opts.Surfaces))}
	}
	surfaces, err := changelogEnums[changelogv1.Surface](opts.Surfaces, changelogv1.Surface_value, "surfaces")
	if err != nil {
		return nil, err
	}
	audiences, err := changelogEnums[changelogv1.Audience](opts.Audiences, changelogv1.Audience_value, "audiences")
	if err != nil {
		return nil, err
	}
	var sort changelogv1.SortDirection
	switch strings.ToLower(strings.TrimSpace(opts.Sort)) {
	case "":
	case "desc":
		sort = changelogv1.SortDirection_SORT_DESC
	case "asc":
		sort = changelogv1.SortDirection_SORT_ASC
	default:
		return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("sort must be \"asc\" or \"desc\", got %q", opts.Sort)}
	}
	req := &changelogv1.ListChangesRequest{
		Categories:     categories,
		ActionRequired: opts.ActionRequired,
		Surfaces:       surfaces,
		Audiences:      audiences,
		PostSlug:       opts.PostSlug,
		Sort:           sort,
		Limit:          limit,
		PageToken:      opts.PageToken,
	}
	if opts.StartTime != nil {
		req.StartTime = timestamppb.New(*opts.StartTime)
	}
	if opts.EndTime != nil {
		req.EndTime = timestamppb.New(*opts.EndTime)
	}
	return req, nil
}

// maxChangelogSurfaces is how many surfaces ListChanges accepts, one per
// surface (ASSISTANT made it 11).
const maxChangelogSurfaces = 11

// changelogEnums maps filter labels such as "breaking_change" to enum values.
// A label this SDK does not know cannot be sent, so it is a ValidationError.
func changelogEnums[E ~int32](labels []string, values map[string]int32, field string) ([]E, error) {
	out := make([]E, 0, len(labels))
	for _, label := range labels {
		value, ok := values[strings.ToUpper(strings.TrimSpace(label))]
		if !ok || value == 0 {
			return nil, &sdkerrors.ValidationError{Msg: fmt.Sprintf("%s: unknown value %q", field, label)}
		}
		out = append(out, E(value))
	}
	return out, nil
}
