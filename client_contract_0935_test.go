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
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1/systemv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	resp := &authv1.ListAccountActivityResponse{Events: []*authv1.ActivityEvent{fullActivityEvent()}}
	if req.Msg.GetPageToken() == "" {
		resp.NextPageToken = "p2"
	}
	return connect.NewResponse(resp), nil
}

// fullActivityEvent is a failed wallet sign-in of a sub-account with every
// field the contract carries set.
func fullActivityEvent() *authv1.ActivityEvent {
	return &authv1.ActivityEvent{
		CreatedAt:      timestamppb.New(activityAt),
		EntityKind:     authv1.ActivityEntityKind_ACTIVITY_ENTITY_SESSION,
		EventAction:    authv1.ActivityEventAction_ACTIVITY_ACTION_FAILED,
		Source:         authv1.ActivityEventSource_ACTIVITY_SOURCE_WEB,
		Ip:             "203.0.113.7",
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64)",
		ActorAccountId: 9000000003,
		PayloadJson:    `{"method":"wallet","wallet":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed","reason":"invalid_signature"}`,
		SubaccountId:   9000000001,
		ActorApiKeyId:  42,
	}
}

var activityAt = time.Date(2026, 11, 17, 7, 1, 2, 345678901, time.UTC)

// wantActivityEvent is fullActivityEvent as the SDK returns it.
func wantActivityEvent() models.SubAccountActivityEvent {
	at := activityAt
	return models.SubAccountActivityEvent{
		EventType:      "session:failed",
		TsMs:           activityAt.UnixMilli(),
		CreatedAt:      &at,
		Source:         "web",
		IP:             "203.0.113.7",
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64)",
		ActorAccountID: codecs.FormatID(9000000003),
		PayloadJSON:    `{"method":"wallet","wallet":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed","reason":"invalid_signature"}`,
		SubaccountID:   codecs.FormatID(9000000001),
		ActorAPIKeyID:  codecs.FormatID(42),
	}
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
	// Every field of the event is kept: the time exactly, the source, the
	// client IP and user agent, the actor's account and API key and the
	// sub-account as public IDs, and the payload JSON raw.
	if len(list.Events) != 1 || !reflect.DeepEqual(list.Events[0], wantActivityEvent()) || list.NextPageToken != "p2" {
		t.Fatalf("list = %+v, want the event %+v", list, wantActivityEvent())
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
	for event, err := range client.SubAccounts.AccountActivity(context.Background(), models.ListAccountActivityOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(event, wantActivityEvent()) {
			t.Fatalf("iterated event = %+v", event)
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
