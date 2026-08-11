package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
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

func (s *SubAccountsService) List(ctx context.Context) (models.SubAccountsList, error) {
	return UnaryAuth(ctx, s.transport, s.writeClient().ListSubaccounts, &authv1.ListSubaccountsRequest{}, decode.SubaccountsListFromProto)
}

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
	req := &authv1.GetSubaccountRequest{SubaccountId: *parsed, IncludeApiKeys: includeAPIKeys, IncludeMembers: includeMembers, IncludeInvites: includeInvites, IncludePolicy: includePolicy, IncludeBalances: includeBalances, InvitesDirection: invitesDirection}
	return UnaryAuth(ctx, s.transport, s.viewClient().GetSubaccount, req, decode.GetSubaccountFromProto)
}

func (s *SubAccountsService) ListMembers(ctx context.Context, account AccountScope, subAccountID *string) (models.SubAccountMembersList, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return models.SubAccountMembersList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().ListSubaccountMembers, &authv1.ListSubaccountMembersRequest{SubaccountId: parsed}, decode.SubaccountMembersListFromProto)
}

func (s *SubAccountsService) ListInvites(ctx context.Context, direction string) (models.SubAccountInvitesList, error) {
	return UnaryAuth(ctx, s.transport, s.writeClient().ListSubaccountInvites, &authv1.ListSubaccountInvitesRequest{Direction: direction}, decode.SubaccountInvitesListFromProto)
}

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
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.viewClient().ListSubaccountActivity, req, decode.SubaccountActivityListFromProto)
}

func (s *SubAccountsService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.SubAccount], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:subaccounts:{account_id}:proto", accountID, s.defaultAccountID, decode.SubaccountFromBytes)
}

func (s *SubAccountsService) SubscribeAPIKeys(ctx context.Context, accountID any) (*realtime.Subscription[models.ApiKeySummary], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:api-keys:{account_id}:proto", accountID, s.defaultAccountID, decode.ApiKeyFromBytes)
}
