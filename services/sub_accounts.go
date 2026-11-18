package services

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SubAccountsService struct {
	transport        *transport.Factory
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewSubAccountsService(factory *transport.Factory, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *SubAccountsService {
	return &SubAccountsService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *SubAccountsService) writeClient() authv1connect.SubaccountServiceClient {
	return authv1connect.NewSubaccountServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *SubAccountsService) viewClient() authv1connect.SubaccountViewServiceClient {
	return authv1connect.NewSubaccountViewServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *SubAccountsService) requireSubaccountID(account AccountScope, subAccountID *string) (uint64, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return 0, err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return 0, err
	}
	if parsed == nil {
		return 0, &errors.ValidationError{Msg: "sub_account_id is required"}
	}
	return *parsed, nil
}

// List returns the sub-accounts in the caller's scope. An API key needs
// READ_SUBACCOUNTS on its policy; a key scoped to the main account sees the
// sub-accounts it owns, a key scoped to a sub-account only that one.
func (s *SubAccountsService) List(ctx context.Context) (models.SubAccountsList, error) {
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().ListSubaccounts, &authv1.ListSubaccountsRequest{}, decode.SubaccountsListFromProto)
}

// Get reads the sub-account's aggregated view. An API key needs READ_SUBACCOUNTS on its
// policy; the members and invites expansions need READ_SUBACCOUNT_MEMBERS and
// READ_SUBACCOUNT_INVITES, and are left out, rather than failing the call,
// for a key without them. A sub-account outside the key's scope is not_found.
// includeBalances also returns
// its current balances in Balances, in the shape of Balances.List. That needs
// the READ_BALANCES action on the sub-account (its role, its attached policy
// and, for an API key, the key's policy), or the call fails with
// PermissionDenied (AUTH_SUBACCOUNT_ACCESS_DENIED). When balances cannot be
// read right now it fails with AUTH_TEMPORARILY_UNAVAILABLE, which
// errors.IsRetryable reports: retry, or read without balances.
func (s *SubAccountsService) Get(ctx context.Context, account AccountScope, subAccountID *string, includeAPIKeys, includeMembers, includeInvites, includePolicy, includeBalances bool, invitesDirection string) (models.GetSubaccountResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.GetSubaccountResult{}, err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return models.GetSubaccountResult{}, err
	}
	if parsed == nil {
		return models.GetSubaccountResult{}, &errors.ValidationError{Msg: "sub_account_id is required"}
	}
	direction, err := codecs.InviteDirectionFromLabel(invitesDirection)
	if err != nil {
		return models.GetSubaccountResult{}, err
	}
	req := &authv1.GetSubaccountRequest{SubaccountId: *parsed, IncludeApiKeys: includeAPIKeys, IncludeMembers: includeMembers, IncludeInvites: includeInvites, IncludePolicy: includePolicy, IncludeBalances: includeBalances, InvitesDirection: direction}
	return UnaryAuthDecoded(ctx, s.transport, s.viewClient().GetSubaccount, req, decode.GetSubaccountFromProto)
}

// ListMembers lists a sub-account's members. It accepts API keys with
// READ_SUBACCOUNT_MEMBERS on their policy, as well as sessions.
func (s *SubAccountsService) ListMembers(ctx context.Context, account AccountScope, subAccountID *string) (models.SubAccountMembersList, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return models.SubAccountMembersList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().ListSubaccountMembers, &authv1.ListSubaccountMembersRequest{SubaccountId: parsed}, decode.SubaccountMembersListFromProto)
}

// ListInvites lists sub-account invites. An API key needs
// READ_SUBACCOUNT_INVITES on its policy.
func (s *SubAccountsService) ListInvites(ctx context.Context, direction string) (models.SubAccountInvitesList, error) {
	parsedDirection, err := codecs.InviteDirectionFromLabel(direction)
	if err != nil {
		return models.SubAccountInvitesList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.writeClient().ListSubaccountInvites, &authv1.ListSubaccountInvitesRequest{Direction: parsedDirection}, decode.SubaccountInvitesListFromProto)
}

// ListActivity lists a sub-account's activity. An API key needs
// READ_SUBACCOUNT_ACTIVITY on its policy.
func (s *SubAccountsService) ListActivity(ctx context.Context, account AccountScope, subAccountID *string, limit int, pageToken *string) (models.SubAccountActivityList, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return models.SubAccountActivityList{}, err
	}
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.SubAccountActivityList{}, err
	}
	req := &authv1.ListSubaccountEventsRequest{SubaccountId: parsed, Limit: parsedLimit}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.SubAccountActivityList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.viewClient().ListSubaccountActivity, req, decode.SubaccountActivityListFromProto)
}

// ListAccountActivity lists the security and activity events of the main
// account and every sub-account in one feed, newest first: sign-ins, API
// keys, MFA, policies, members, invites and saved destinations. Only the main
// account's own session, or an API key with READ_ACCOUNT_ACTIVITY, may call
// it; a key scoped to a sub-account reads only that sub-account's events and
// is refused IncludeMainAccount. Filters the contract does not take (more
// than 100 sub-accounts or a repeated one, an unknown or repeated label, too
// many labels, an EndTime not after StartTime, a limit above 200) are
// refused with an *errors.ValidationError before sending.
func (s *SubAccountsService) ListAccountActivity(ctx context.Context, opts models.ListAccountActivityOptions) (models.SubAccountActivityList, error) {
	req, err := accountActivityRequest(opts)
	if err != nil {
		return models.SubAccountActivityList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.readClient().ListAccountActivity, req, decode.AccountActivityListFromProto)
}

// AccountActivity iterates over ListAccountActivity from opts.PageToken (or
// the newest event), following each page's NextPageToken with the same
// filters. It stops at the first error.
func (s *SubAccountsService) AccountActivity(ctx context.Context, opts models.ListAccountActivityOptions) iter.Seq2[models.SubAccountActivityEvent, error] {
	return paginate(opts.PageToken, func(token string) ([]models.SubAccountActivityEvent, string, error) {
		page := opts
		page.PageToken = token
		list, err := s.ListAccountActivity(ctx, page)
		return list.Events, list.NextPageToken, err
	})
}

func (s *SubAccountsService) readClient() authv1connect.AccountActivityServiceClient {
	return authv1connect.NewAccountActivityServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func accountActivityRequest(opts models.ListAccountActivityOptions) (*authv1.ListAccountActivityRequest, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return nil, err
	}
	req := &authv1.ListAccountActivityRequest{Limit: limit, PageToken: opts.PageToken, IncludeMainAccount: opts.IncludeMainAccount}
	if err := codecs.CheckPageLimit(req); err != nil {
		return nil, err
	}
	if err := codecs.CheckContractRepeated(req, "subaccount_ids", len(opts.SubAccountIDs), "subaccount_ids"); err != nil {
		return nil, err
	}
	seen := make(map[uint64]bool, len(opts.SubAccountIDs))
	for i, text := range opts.SubAccountIDs {
		id, err := codecs.ParseAccountID(text, fmt.Sprintf("subaccount_ids[%d]", i))
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, &errors.ValidationError{Msg: fmt.Sprintf("subaccount_ids[%d] repeats a sub-account", i)}
		}
		seen[id] = true
		req.SubaccountIds = append(req.SubaccountIds, id)
	}
	if req.EntityKinds, err = activityLabels[authv1.ActivityEntityKind](req, "entity_kinds", opts.EntityKinds, "ACTIVITY_ENTITY_", authv1.ActivityEntityKind_value); err != nil {
		return nil, err
	}
	if req.EventActions, err = activityLabels[authv1.ActivityEventAction](req, "event_actions", opts.EventActions, "ACTIVITY_ACTION_", authv1.ActivityEventAction_value); err != nil {
		return nil, err
	}
	if req.Sources, err = activityLabels[authv1.ActivityEventSource](req, "sources", opts.Sources, "ACTIVITY_SOURCE_", authv1.ActivityEventSource_value); err != nil {
		return nil, err
	}
	if opts.APIKeyID != nil {
		if req.ApiKeyId, err = codecs.ParseAccountID(*opts.APIKeyID, "api_key_id"); err != nil {
			return nil, err
		}
	}
	if opts.StartTime != nil {
		req.StartTime = timestamppb.New(*opts.StartTime)
	}
	if opts.EndTime != nil {
		req.EndTime = timestamppb.New(*opts.EndTime)
	}
	if opts.StartTime != nil && opts.EndTime != nil && !opts.EndTime.After(*opts.StartTime) {
		return nil, &errors.ValidationError{Msg: "end_time must be after start_time"}
	}
	return req, nil
}

// activityLabels converts activity filter labels, with or without their
// enum prefix and in any case, refusing an unknown, unspecified or repeated
// one and more than the contract's max_items.
func activityLabels[E ~int32](req *authv1.ListAccountActivityRequest, field protoreflect.Name, labels []string, prefix string, values map[string]int32) ([]E, error) {
	if err := codecs.CheckContractRepeated(req, field, len(labels), string(field)); err != nil {
		return nil, err
	}
	out := make([]E, 0, len(labels))
	seen := make(map[int32]bool, len(labels))
	for i, label := range labels {
		name := strings.ToUpper(label)
		if !strings.HasPrefix(name, prefix) {
			name = prefix + name
		}
		value, ok := values[name]
		if !ok || value == 0 {
			return nil, &errors.ValidationError{Msg: fmt.Sprintf("%s[%d]: unknown value %q", field, i, label)}
		}
		if seen[value] {
			return nil, &errors.ValidationError{Msg: fmt.Sprintf("%s[%d] repeats %q", field, i, label)}
		}
		seen[value] = true
		out = append(out, E(value))
	}
	return out, nil
}

func (s *SubAccountsService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.SubAccount], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:subaccounts:{account_id}:proto", accountID, s.defaultAccountID, decode.SubaccountFromBytes)
}

func (s *SubAccountsService) SubscribeAPIKeys(ctx context.Context, accountID any) (*realtime.Subscription[models.ApiKeySummary], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:api-keys:{account_id}:proto", accountID, s.defaultAccountID, decode.ApiKeyFromBytes)
}
