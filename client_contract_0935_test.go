package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1/systemv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

type fakeClock struct {
	systemv1connect.UnimplementedSystemServiceHandler

	mu     sync.Mutex
	signed bool
	timeMs int64
}

func (f *fakeClock) GetTime(_ context.Context, req *connect.Request[systemv1.GetTimeRequest]) (*connect.Response[systemv1.GetTimeResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := req.Header()
	if h.Get("Authorization") != "" || h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
		f.signed = true
	}
	return connect.NewResponse(&systemv1.GetTimeResponse{ServerTimeMs: f.timeMs}), nil
}

type fakeActivity struct {
	authv1connect.UnimplementedAccountActivityServiceHandler

	mu       sync.Mutex
	requests []*authv1.ListAccountActivityRequest
	unsigned int
}

func (f *fakeActivity) ListAccountActivity(_ context.Context, req *connect.Request[authv1.ListAccountActivityRequest]) (*connect.Response[authv1.ListAccountActivityResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.Header().Get("X-API-KEY-ID") == "" || req.Header().Get("X-API-SIGNATURE") == "" {
		f.unsigned++
	}
	f.requests = append(f.requests, proto.Clone(req.Msg).(*authv1.ListAccountActivityRequest))
	resp := &authv1.ListAccountActivityResponse{Events: []*authv1.ActivityEvent{{
		EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_SESSION, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_CREATED,
	}}}
	if req.Msg.GetPageToken() == "" {
		resp.NextPageToken = "p2"
	}
	return connect.NewResponse(resp), nil
}

func newContract0935Client(t *testing.T, handlers ...func(*http.ServeMux)) *Client {
	t.Helper()
	mux := http.NewServeMux()
	for _, register := range handlers {
		register(mux)
	}
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_0935", APIPrivateKey: keypair.SecretKeyHex, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// GetServerTime reads the server clock unsigned and estimates this host's
// offset from it; a reply without a server time breaks the contract.
func TestGetServerTimeEstimatesTheClockOffset(t *testing.T) {
	ahead := time.Now().Add(90 * time.Second).UnixMilli()
	fake := &fakeClock{timeMs: ahead}
	client := newContract0935Client(t, func(mux *http.ServeMux) {
		path, handler := systemv1connect.NewSystemServiceHandler(fake)
		mux.Handle(path, handler)
	})
	got, err := client.GetServerTime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Time.Equal(time.UnixMilli(ahead)) || got.Time.Location() != time.UTC {
		t.Fatalf("time = %v", got.Time)
	}
	if got.Offset < 89*time.Second || got.Offset > 91*time.Second || got.RoundTrip < 0 {
		t.Fatalf("offset = %v round trip = %v, want about 90s", got.Offset, got.RoundTrip)
	}
	if fake.signed {
		t.Fatal("GetTime was signed")
	}
	fake.timeMs = 0
	var contract *sdkerrors.ResponseContractError
	if _, err := client.GetServerTime(context.Background()); !stderrors.As(err, &contract) {
		t.Fatalf("missing server time: err = %v", err)
	}
}

// ListAccountActivity sends its filters as the contract takes them, signed,
// and refuses what the contract refuses before sending.
func TestListAccountActivitySendsItsFilters(t *testing.T) {
	fake := &fakeActivity{}
	client := newContract0935Client(t, func(mux *http.ServeMux) {
		path, handler := authv1connect.NewAccountActivityServiceHandler(fake)
		mux.Handle(path, handler)
	})
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 17)
	key := codecs.FormatID(42)
	opts := models.ListAccountActivityOptions{
		SubAccountIDs:      []string{codecs.FormatID(9000000001), codecs.FormatID(9000000002)},
		IncludeMainAccount: true,
		EntityKinds:        []string{"session", "ACTIVITY_ENTITY_API_KEY"},
		EventActions:       []string{"failed"},
		Sources:            []string{"WEB"},
		APIKeyID:           &key,
		StartTime:          &start,
		EndTime:            &end,
		Limit:              200,
	}
	list, err := client.SubAccounts.ListAccountActivity(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Events) != 1 || list.Events[0].EventType != "session:created" || list.NextPageToken != "p2" {
		t.Fatalf("list = %+v", list)
	}
	want := &authv1.ListAccountActivityRequest{
		Limit: 200, SubaccountIds: []uint64{9000000001, 9000000002}, IncludeMainAccount: true,
		EntityKinds:  []authv1.ActivityEntityKind{authv1.ActivityEntityKind_ACTIVITY_ENTITY_SESSION, authv1.ActivityEntityKind_ACTIVITY_ENTITY_API_KEY},
		EventActions: []authv1.ActivityEventAction{authv1.ActivityEventAction_ACTIVITY_ACTION_FAILED},
		Sources:      []authv1.ActivityEventSource{authv1.ActivityEventSource_ACTIVITY_SOURCE_WEB},
		ApiKeyId:     42,
	}
	sent := fake.requests[0]
	if !sent.GetStartTime().AsTime().Equal(start) || !sent.GetEndTime().AsTime().Equal(end) {
		t.Fatalf("time range = %v..%v", sent.GetStartTime(), sent.GetEndTime())
	}
	sent.StartTime, sent.EndTime = nil, nil
	if !proto.Equal(sent, want) || fake.unsigned != 0 {
		t.Fatalf("request = %v, want %v (unsigned=%d)", sent, want, fake.unsigned)
	}

	// The iterator follows the page token with the same filters.
	var events int
	for _, err := range client.SubAccounts.AccountActivity(context.Background(), models.ListAccountActivityOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		events++
	}
	if events != 2 || fake.requests[2].GetPageToken() != "p2" {
		t.Fatalf("events = %d, second page token = %q", events, fake.requests[2].GetPageToken())
	}

	sentBefore := len(fake.requests)
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = codecs.FormatID(uint64(i + 1))
	}
	empty := ""
	for name, bad := range map[string]models.ListAccountActivityOptions{
		"limit over 200":          {Limit: 201},
		"101 sub-accounts":        {SubAccountIDs: tooMany},
		"repeated sub-account":    {SubAccountIDs: []string{codecs.FormatID(7), codecs.FormatID(7)}},
		"empty sub-account":       {SubAccountIDs: []string{""}},
		"unknown entity kind":     {EntityKinds: []string{"wallet"}},
		"unspecified action":      {EventActions: []string{"unspecified"}},
		"repeated source":         {Sources: []string{"web", "WEB"}},
		"empty api key":           {APIKeyID: &empty},
		"end not after the start": {StartTime: &start, EndTime: &start},
	} {
		var validation *sdkerrors.ValidationError
		if _, err := client.SubAccounts.ListAccountActivity(context.Background(), bad); !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %v, want a ValidationError", name, err)
		}
	}
	if len(fake.requests) != sentBefore {
		t.Fatalf("%d refused requests were sent", len(fake.requests)-sentBefore)
	}
}
