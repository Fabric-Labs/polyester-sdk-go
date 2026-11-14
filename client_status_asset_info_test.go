package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1/systemv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeStatusEvents struct {
	systemv1connect.UnimplementedSystemServiceHandler

	mu       sync.Mutex
	requests []*systemv1.ListStatusEventsRequest
	signed   int
	pages    map[string]*systemv1.ListStatusEventsResponse
}

func (f *fakeStatusEvents) ListStatusEvents(_ context.Context, req *connect.Request[systemv1.ListStatusEventsRequest]) (*connect.Response[systemv1.ListStatusEventsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, proto.Clone(req.Msg).(*systemv1.ListStatusEventsRequest))
	if h := req.Header(); h.Get("Authorization") != "" || h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
		f.signed++
	}
	return connect.NewResponse(f.pages[req.Msg.GetPageToken()]), nil
}

type fakeAssetInfo struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler

	mu       sync.Mutex
	requests []*marketdatav1.ListAssetInfoRequest
	signed   int
	resp     *marketdatav1.ListAssetInfoResponse
}

func (f *fakeAssetInfo) ListAssetInfo(_ context.Context, req *connect.Request[marketdatav1.ListAssetInfoRequest]) (*connect.Response[marketdatav1.ListAssetInfoResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, proto.Clone(req.Msg).(*marketdatav1.ListAssetInfoRequest))
	if h := req.Header(); h.Get("Authorization") != "" || h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
		f.signed++
	}
	return connect.NewResponse(f.resp), nil
}

// newPublicReadClient serves handlers on a loopback server for a client
// configured with credentials, so a signed request would show.
func newPublicReadClient(t *testing.T, register func(*http.ServeMux)) *Client {
	t.Helper()
	mux := http.NewServeMux()
	register(mux)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_public", APIPrivateKey: keypair.SecretKeyHex})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestStatusEventsAreReadUnsignedAndResendTheFiltersWithEachToken(t *testing.T) {
	start := time.Date(2026, 11, 12, 19, 30, 0, 0, time.UTC)
	noted := time.Date(2026, 11, 12, 19, 45, 0, 0, time.UTC)
	details := "https://example.com/announcements/sol-deposits"
	fake := &fakeStatusEvents{pages: map[string]*systemv1.ListStatusEventsResponse{
		"": {
			Events: []*systemv1.StatusEvent{{
				Id:         42,
				Kind:       systemv1.StatusEventKind_INCIDENT,
				State:      systemv1.StatusEventState_IN_PROGRESS,
				Title:      "SOL deposits delayed",
				Message:    "SOL deposits are taking longer than usual to be credited.",
				StartTime:  timestamppb.New(start),
				Services:   []systemv1.AffectedService{systemv1.AffectedService_DEPOSITS, systemv1.AffectedService_QUICKSWAP},
				Assets:     []string{"SOL"},
				Impact:     systemv1.StatusEventImpact_DEGRADED,
				UpdatedAt:  timestamppb.New(noted),
				Updates:    []*systemv1.StatusEventUpdate{{Id: 1, Message: "The cause is identified.", CreatedAt: timestamppb.New(noted)}},
				DetailsUrl: &details,
			}},
			NextPageToken: "p2",
		},
		"p2": {Events: []*systemv1.StatusEvent{{
			Id:       43,
			Kind:     systemv1.StatusEventKind_MAINTENANCE,
			State:    systemv1.StatusEventState_SCHEDULED,
			Services: []systemv1.AffectedService{systemv1.AffectedService(99)},
		}}},
	}}
	client := newPublicReadClient(t, func(mux *http.ServeMux) {
		path, handler := systemv1connect.NewSystemServiceHandler(fake)
		mux.Handle(path, handler)
	})

	opts := models.ListStatusEventsOptions{States: []string{"scheduled", "IN_PROGRESS"}, Kinds: []string{models.StatusEventKindIncident, "maintenance"}, Limit: 20}
	var events []models.StatusEvent
	for event, err := range client.StatusEvents(context.Background(), opts) {
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	want := models.StatusEvent{
		ID: codecs.FormatID(42), Kind: "incident", State: "in_progress",
		Title: "SOL deposits delayed", Message: "SOL deposits are taking longer than usual to be credited.",
		StartTime: &start, Services: []string{"deposits", "quickswap"}, Assets: []string{"SOL"},
		Impact: models.StatusEventImpactDegraded, UpdatedAt: &noted,
		Updates:    []models.StatusEventUpdate{{ID: 1, Message: "The cause is identified.", CreatedAt: &noted}},
		DetailsURL: &details,
	}
	if !reflect.DeepEqual(events[0], want) {
		t.Fatalf("event = %+v\nwant    %+v", events[0], want)
	}
	// Omitted optional values stay nil, an unspecified impact reads "", and a
	// service this SDK does not know keeps its number.
	second := events[1]
	if second.EndTime != nil || second.CancelledAt != nil || second.DetailsURL != nil || second.Impact != "" ||
		second.Kind != models.StatusEventKindMaintenance || second.State != models.StatusEventStateScheduled ||
		!reflect.DeepEqual(second.Services, []string{"unknown(99)"}) {
		t.Fatalf("second event = %+v", second)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 2 || fake.signed != 0 {
		t.Fatalf("requests=%d signed=%d; want two unsigned requests", len(fake.requests), fake.signed)
	}
	wantStates := []systemv1.StatusEventState{systemv1.StatusEventState_SCHEDULED, systemv1.StatusEventState_IN_PROGRESS}
	wantKinds := []systemv1.StatusEventKind{systemv1.StatusEventKind_INCIDENT, systemv1.StatusEventKind_MAINTENANCE}
	for i, req := range fake.requests {
		if !reflect.DeepEqual(req.GetStates(), wantStates) || !reflect.DeepEqual(req.GetKinds(), wantKinds) || req.GetLimit() != 20 {
			t.Fatalf("request %d = %v; want the same states, kinds and limit on every page", i, req)
		}
	}
	if fake.requests[1].GetPageToken() != "p2" {
		t.Fatalf("second page token = %q", fake.requests[1].GetPageToken())
	}
}

func TestListStatusEventsLeavesTheDefaultStatesToTheServer(t *testing.T) {
	fake := &fakeStatusEvents{pages: map[string]*systemv1.ListStatusEventsResponse{"": {}}}
	client := newPublicReadClient(t, func(mux *http.ServeMux) {
		path, handler := systemv1connect.NewSystemServiceHandler(fake)
		mux.Handle(path, handler)
	})
	page, err := client.ListStatusEvents(context.Background(), models.ListStatusEventsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Events == nil || len(page.Events) != 0 || page.NextPageToken != "" {
		t.Fatalf("page = %+v; want an empty, non-nil event list", page)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 1 || len(fake.requests[0].GetStates()) != 0 || len(fake.requests[0].GetKinds()) != 0 {
		t.Fatalf("requests = %v; want empty states and kinds", fake.requests)
	}
}

func TestListStatusEventsRefusesUnknownLabelsBeforeSending(t *testing.T) {
	fake := &fakeStatusEvents{}
	client := newPublicReadClient(t, func(mux *http.ServeMux) {
		path, handler := systemv1connect.NewSystemServiceHandler(fake)
		mux.Handle(path, handler)
	})
	for _, opts := range []models.ListStatusEventsOptions{
		{States: []string{"ongoing"}},
		{States: []string{"state_unspecified"}},
		{Kinds: []string{"outage"}},
		{Limit: -1},
	} {
		_, err := client.ListStatusEvents(context.Background(), opts)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("ListStatusEvents(%+v) error = %v, want *errors.ValidationError", opts, err)
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("sent %d requests, want none", len(fake.requests))
	}
}

func TestListAssetInfoIsUnsignedAndKeepsUnknownApartFromZero(t *testing.T) {
	updated := time.Date(2026, 11, 12, 21, 11, 0, 0, time.UTC)
	fake := &fakeAssetInfo{resp: &marketdatav1.ListAssetInfoResponse{AssetInfo: []*marketdatav1.AssetInfo{
		{
			AssetId: 1,
			Editorial: &marketdatav1.AssetEditorialInfo{
				Description:  "Bitcoin is a decentralized digital currency.",
				WebsiteUrl:   "https://bitcoin.org",
				ExplorerUrls: []string{"https://mempool.space", "https://blockstream.info"},
				GithubUrl:    "https://github.com/bitcoin/bitcoin",
				IssueDate:    "2009-01-03",
				IssuePrice:   &marketdatav1.AssetIssuePrice{Amount: "0", Currency: "USD"},
			},
			EditorialUpdatedAt: timestamppb.New(updated),
		},
		// Cleared information keeps an empty entry; no price is unknown.
		{AssetId: 2, Editorial: &marketdatav1.AssetEditorialInfo{}},
		{AssetId: 3},
	}}}
	client := newPublicReadClient(t, func(mux *http.ServeMux) {
		path, handler := marketdatav1connect.NewMarketDataServiceHandler(fake)
		mux.Handle(path, handler)
	})
	got, err := client.MarketData.ListAssetInfo(context.Background(), 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.AssetInfo{
		{
			AssetID: 1,
			Editorial: models.AssetEditorialInfo{
				Description:  "Bitcoin is a decentralized digital currency.",
				WebsiteURL:   "https://bitcoin.org",
				ExplorerURLs: []string{"https://mempool.space", "https://blockstream.info"},
				GitHubURL:    "https://github.com/bitcoin/bitcoin",
				IssueDate:    "2009-01-03",
				IssuePrice:   &models.AssetIssuePrice{Amount: "0", Currency: "USD"},
			},
			EditorialUpdatedAt: &updated,
		},
		{AssetID: 2},
		{AssetID: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListAssetInfo = %+v\nwant %+v", got, want)
	}

	if _, err := client.MarketData.ListAssetInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.signed != 0 || len(fake.requests) != 2 {
		t.Fatalf("requests=%d signed=%d; want two unsigned requests", len(fake.requests), fake.signed)
	}
	if !reflect.DeepEqual(fake.requests[0].GetAssetIds(), []uint32{1, 2, 3}) || len(fake.requests[1].GetAssetIds()) != 0 {
		t.Fatalf("asset_ids = %v then %v", fake.requests[0].GetAssetIds(), fake.requests[1].GetAssetIds())
	}
}
