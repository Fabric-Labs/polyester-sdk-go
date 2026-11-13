package services

import (
	"context"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SubAccounts.Get returns the sub-account's members, invites and attached
// policy when they are asked for and the server sends them, read as
// ListMembers, ListInvites and the policy stream read them. Every read goes
// through the public service over both wire formats.

const expansionSubaccount = uint64(42)

var expansionReviewAt = time.Date(2026, 12, 1, 9, 30, 0, 250_000_000, time.UTC)

func expansionMembers() []*authv1.SubaccountMemberView {
	return []*authv1.SubaccountMemberView{
		{AccountId: 7, Role: authv1.SubaccountRole_OWNER, Username: "alice"},
		{AccountId: 9, Role: authv1.SubaccountRole_TRADER, Username: "bob"},
	}
}

func expansionInvites() []*authv1.SubaccountInvite {
	return []*authv1.SubaccountInvite{{
		Id: 11, SubaccountId: expansionSubaccount, GranteeAccountId: 12, InviterAccountId: 7,
		Role: authv1.SubaccountRole_VIEWER, Status: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_PENDING,
	}}
}

func expansionPolicy() *authv1.SubaccountPolicyView {
	return &authv1.SubaccountPolicyView{
		Id: 5, Name: "desk", Description: "desk limits",
		SpotMarkets:      []*authv1.SpotMarketRule{{SymbolId: 1}},
		SpotMarketScope:  authv1.MarketScope_ALLOWLIST,
		Actions:          []authv1.PolicyAction{authv1.PolicyAction_TRADE_SPOT, authv1.PolicyAction_READ_BALANCES},
		MaxOrderNotional: 1_000_000, MaxOpenOrders: 20, TradingHalted: true, Locked: true,
		ReviewAt: timestamppb.New(expansionReviewAt), Revision: 4,
	}
}

type expansionHandler struct {
	authv1connect.UnimplementedSubaccountViewServiceHandler
	// withPolicy is false for a sub-account with no attached policy.
	withPolicy bool
	mu         sync.Mutex
	requests   []*authv1.GetSubaccountRequest
}

// GetSubaccount answers each expansion only when it was asked for, as the
// server does.
func (h *expansionHandler) GetSubaccount(_ context.Context, req *connect.Request[authv1.GetSubaccountRequest]) (*connect.Response[authv1.GetSubaccountResponse], error) {
	h.mu.Lock()
	h.requests = append(h.requests, req.Msg)
	h.mu.Unlock()
	resp := &authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: req.Msg.GetSubaccountId()}}
	if req.Msg.GetIncludeMembers() {
		resp.Members = expansionMembers()
	}
	if req.Msg.GetIncludeInvites() {
		resp.Invites = expansionInvites()
	}
	if req.Msg.GetIncludePolicy() && h.withPolicy {
		resp.Policy = expansionPolicy()
	}
	return connect.NewResponse(resp), nil
}

func newExpansionService(t *testing.T, wire string, handler *expansionHandler) *SubAccountsService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := authv1connect.NewSubaccountViewServiceHandler(handler)
	mux.Handle(path, h)
	server := hardening.NewTestServer(mux)
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	return NewSubAccountsService(transport.NewFactory(cfg, creds, nil), nil, nil, nil)
}

func TestSubAccountsGetReturnsMembersInvitesAndPolicyWhenAsked(t *testing.T) {
	for _, wire := range retentionWires {
		t.Run(wire, func(t *testing.T) {
			handler := &expansionHandler{withPolicy: true}
			svc := newExpansionService(t, wire, handler)
			ctx := context.Background()
			subID := codecs.FormatID(expansionSubaccount)

			plain, err := svc.Get(ctx, nil, &subID, false, false, false, false, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if plain.Members != nil || plain.Invites != nil || plain.Policy != nil {
				t.Fatalf("expansions without asking: members=%+v invites=%+v policy=%+v", plain.Members, plain.Invites, plain.Policy)
			}

			got, err := svc.Get(ctx, nil, &subID, false, true, true, true, false, "incoming")
			if err != nil {
				t.Fatal(err)
			}
			if got.Subaccount == nil || got.Subaccount.SubaccountID != subID {
				t.Fatalf("Subaccount = %+v", got.Subaccount)
			}

			wantMembers := []models.SubAccountMember{
				{GranteeAccountID: codecs.FormatID(7), Role: "owner"},
				{GranteeAccountID: codecs.FormatID(9), Role: "trader"},
			}
			if !reflect.DeepEqual(got.Members, wantMembers) {
				t.Fatalf("Members = %+v, want %+v", got.Members, wantMembers)
			}
			// The same decoders as ListMembers and ListInvites.
			listedMembers := decode.SubaccountMembersListFromProto(&authv1.ListSubaccountMembersResponse{Members: expansionMembers()})
			if !reflect.DeepEqual(got.Members, listedMembers.Members) {
				t.Fatalf("Get members %+v != ListMembers %+v", got.Members, listedMembers.Members)
			}
			wantInvites := []models.SubAccountInvite{{
				InviteID: codecs.FormatID(11), GranteeAccountID: codecs.FormatID(12),
				Role: "viewer", Status: "SUBACCOUNT_INVITE_STATUS_PENDING",
			}}
			if !reflect.DeepEqual(got.Invites, wantInvites) {
				t.Fatalf("Invites = %+v, want %+v", got.Invites, wantInvites)
			}
			listedInvites := decode.SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{Invites: expansionInvites()})
			if !reflect.DeepEqual(got.Invites, listedInvites.Invites) {
				t.Fatalf("Get invites %+v != ListInvites %+v", got.Invites, listedInvites.Invites)
			}

			// The same decoder as the sub-account policy stream.
			if got.Policy == nil {
				t.Fatal("Policy missing")
			}
			if want := decode.SubaccountPolicyMessageFromProto(expansionPolicy()); !reflect.DeepEqual(got.Policy, want) {
				t.Fatalf("Policy = %+v, want %+v", got.Policy, want)
			}
			p := got.Policy
			if p.PolicyID != codecs.FormatID(5) || p.Name != "desk" || p.SpotMarketScope != "ALLOWLIST" ||
				!reflect.DeepEqual(p.Actions, []string{"TRADE_SPOT", "READ_BALANCES"}) ||
				p.MaxOrderNotional != 1_000_000 || p.MaxOpenOrders != 20 || !p.TradingHalted || !p.Locked ||
				p.Revision != 4 || len(p.SpotMarkets) != 1 || p.SpotMarkets[0].SymbolID != 1 {
				t.Fatalf("Policy = %+v", p)
			}
			if p.ReviewAt == nil || !p.ReviewAt.Equal(expansionReviewAt) {
				t.Fatalf("ReviewAt = %v, want %v", p.ReviewAt, expansionReviewAt)
			}
			// Unset timestamps stay nil, never the zero time.
			if p.ExpiresAt != nil || p.CreatedAt != nil || p.UpdatedAt != nil {
				t.Fatalf("unset policy timestamps: expires=%v created=%v updated=%v", p.ExpiresAt, p.CreatedAt, p.UpdatedAt)
			}

			handler.mu.Lock()
			defer handler.mu.Unlock()
			if len(handler.requests) != 2 {
				t.Fatalf("requests=%v", handler.requests)
			}
			first, second := handler.requests[0], handler.requests[1]
			if first.GetIncludeMembers() || first.GetIncludeInvites() || first.GetIncludePolicy() {
				t.Fatalf("first request asked for expansions: %v", first)
			}
			if !second.GetIncludeMembers() || !second.GetIncludeInvites() || !second.GetIncludePolicy() ||
				second.GetIncludeApiKeys() || second.GetIncludeBalances() || second.GetSubaccountId() != expansionSubaccount ||
				second.GetInvitesDirection() != authv1.SubaccountInviteDirection_INCOMING {
				t.Fatalf("second request = %v", second)
			}
		})
	}
}

// A sub-account with no attached policy reads Policy as nil even when it was
// asked for, and empty member and invite lists stay nil.
func TestSubAccountsGetWithoutAttachedPolicyReadsNil(t *testing.T) {
	for _, wire := range retentionWires {
		t.Run(wire, func(t *testing.T) {
			svc := newExpansionService(t, wire, &expansionHandler{withPolicy: false})
			subID := codecs.FormatID(expansionSubaccount)
			got, err := svc.Get(context.Background(), nil, &subID, false, false, false, true, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if got.Policy != nil || got.Members != nil || got.Invites != nil {
				t.Fatalf("policy=%+v members=%+v invites=%+v", got.Policy, got.Members, got.Invites)
			}
		})
	}
}
