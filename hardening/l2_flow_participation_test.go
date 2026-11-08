package hardening_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
)

// flowsAPI answers ListFlows with one owned and one received transfer and
// keeps every request it decoded.
type flowsAPI struct {
	mu       sync.Mutex
	requests []*lifecyclev1.ListFlowsRequest
}

func (a *flowsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := &lifecyclev1.ListFlowsRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.requests = append(a.requests, req)
	a.mu.Unlock()
	out, _ := proto.Marshal(&lifecyclev1.ListFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{
		{FlowId: "sent", FlowKind: lifecyclev1.FlowKind_KIND_TRANSFER, OwnerAccountId: 7, Participation: lifecyclev1.FlowParticipation_PARTICIPATION_OWNER,
			SourceAddress: "0x00000000000000000000000000000000000000a1", DestinationAddress: "0x00000000000000000000000000000000000000b2"},
		{FlowId: "received", FlowKind: lifecyclev1.FlowKind_KIND_TRANSFER, OwnerAccountId: 9, Participation: lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT,
			SourceAddress: "0x00000000000000000000000000000000000000c3", DestinationAddress: "0x00000000000000000000000000000000000000a1"},
		{FlowId: "future", Participation: lifecyclev1.FlowParticipation(9)},
	}})
	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(out)
}

func (a *flowsAPI) last(t *testing.T) *lifecyclev1.ListFlowsRequest {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.requests) == 0 {
		t.Fatal("no ListFlows request reached the API")
	}
	return a.requests[len(a.requests)-1]
}

func (a *flowsAPI) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

func flowsClient(t *testing.T) (*polyester.Client, *flowsAPI) {
	t.Helper()
	api := &flowsAPI{}
	srv := hardening.NewTestServer(api)
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{APIURL: srv.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, api
}

// A list that selects an account carries received transfers; each row says
// how the account takes part, with the addresses that name the counterparty.
func TestListFlowsRowsCarryParticipation(t *testing.T) {
	client, api := flowsClient(t)
	owner := "8"
	page, err := client.Lifecycle.ListFlows(context.Background(), 10, nil, nil, &owner, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := api.last(t).GetParticipation(); got != lifecyclev1.FlowParticipation_PARTICIPATION_UNSPECIFIED {
		t.Fatalf("ListFlows participation = %v, want unspecified (both sides)", got)
	}
	if len(page.Flows) != 3 {
		t.Fatalf("flows = %+v", page.Flows)
	}
	sent, received, future := page.Flows[0], page.Flows[1], page.Flows[2]
	if sent.Participation != "owner" || sent.DestinationAddress != "0x00000000000000000000000000000000000000b2" {
		t.Fatalf("sent = %+v", sent)
	}
	if received.Participation != "recipient" || received.SourceAddress != "0x00000000000000000000000000000000000000c3" {
		t.Fatalf("received = %+v", received)
	}
	if future.Participation != "unknown_participation_9" {
		t.Fatalf("a newer participation reads %q", future.Participation)
	}
}

func TestListFlowsWithSendsTheParticipationFilter(t *testing.T) {
	client, api := flowsClient(t)
	owner := "8"
	for label, want := range map[string]lifecyclev1.FlowParticipation{
		"owner":                   lifecyclev1.FlowParticipation_PARTICIPATION_OWNER,
		"RECIPIENT":               lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT,
		"PARTICIPATION_RECIPIENT": lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT,
		"":                        lifecyclev1.FlowParticipation_PARTICIPATION_UNSPECIFIED,
	} {
		participation := label
		if _, err := client.Lifecycle.ListFlowsWith(context.Background(), services.ListFlowsParams{OwnerAccountID: &owner, Participation: &participation}); err != nil {
			t.Fatalf("%q: %v", label, err)
		}
		req := api.last(t)
		if req.GetParticipation() != want || req.GetOwnerAccountId() == 0 || req.GetLimit() != 50 {
			t.Fatalf("%q: request = %v", label, req)
		}
	}
	calls := api.count()
	bad := "sender"
	_, err := client.Lifecycle.ListFlowsWith(context.Background(), services.ListFlowsParams{OwnerAccountID: &owner, Participation: &bad})
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || api.count() != calls {
		t.Fatalf("unknown participation: err = %v, requests %d -> %d", err, calls, api.count())
	}
}
