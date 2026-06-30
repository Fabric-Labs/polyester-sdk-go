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

func (s *SubAccountsService) Create(ctx context.Context, smartAccountAddress, nonce, signature, label, icon, color, primaryWalletAddress, walletProvider string) (models.CreateSubaccountResult, error) {
	req := &authv1.CreateSubaccountRequest{
		Label: label, Icon: icon, Color: color,
		SmartAccountAddress: smartAccountAddress, Nonce: nonce, Signature: signature,
		WalletProvider: walletProvider,
	}
	if primaryWalletAddress != "" {
		req.PrimaryWalletAddress = &primaryWalletAddress
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().CreateSubaccount, req, decode.CreateSubaccountFromProto)
}

func (s *SubAccountsService) Update(ctx context.Context, account AccountScope, subAccountID *string, label, icon, color, status string) error {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return err
	}
	req := &authv1.UpdateSubaccountRequest{SubaccountId: parsed, Label: label, Icon: icon, Color: color, Status: status}
	_, err = UnaryAuth(ctx, s.transport, s.writeClient().UpdateSubaccount, req, decode.Void[authv1.UpdateSubaccountResponse])
	return err
}

func (s *SubAccountsService) Delete(ctx context.Context, account AccountScope, subAccountID *string) error {
	return s.Update(ctx, account, subAccountID, "", "", "", "deleted")
}

func (s *SubAccountsService) SetMemberMFARequirement(ctx context.Context, account AccountScope, subAccountID *string, requireMemberMFA bool) error {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return err
	}
	req := &authv1.SetSubaccountMemberMFARequirementRequest{SubaccountId: parsed, RequireMemberMfa: requireMemberMFA}
	_, err = UnaryAuth(ctx, s.transport, s.writeClient().SetSubaccountMemberMFARequirement, req, decode.Void[authv1.SetSubaccountMemberMFARequirementResponse])
	return err
}

func (s *SubAccountsService) ListMembers(ctx context.Context, account AccountScope, subAccountID *string) (models.SubAccountMembersList, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return models.SubAccountMembersList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().ListSubaccountMembers, &authv1.ListSubaccountMembersRequest{SubaccountId: parsed}, decode.SubaccountMembersListFromProto)
}

func (s *SubAccountsService) RemoveMember(ctx context.Context, account AccountScope, subAccountID *string, granteeAccountID string) error {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return err
	}
	grantee, err := codecs.IDToInt(granteeAccountID, "grantee_account_id")
	if err != nil {
		return err
	}
	req := &authv1.RemoveSubaccountMemberRequest{SubaccountId: parsed, GranteeAccountId: grantee}
	_, err = UnaryAuth(ctx, s.transport, s.writeClient().RemoveSubaccountMember, req, decode.Void[authv1.RemoveSubaccountMemberResponse])
	return err
}

func (s *SubAccountsService) UpdateMemberRole(ctx context.Context, account AccountScope, subAccountID *string, granteeAccountID, role string) error {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return err
	}
	grantee, err := codecs.IDToInt(granteeAccountID, "grantee_account_id")
	if err != nil {
		return err
	}
	protoRole, err := codecs.SubaccountRoleFromLabel(role)
	if err != nil {
		return err
	}
	req := &authv1.UpdateSubaccountMemberRoleRequest{SubaccountId: parsed, GranteeAccountId: grantee, Role: protoRole}
	_, err = UnaryAuth(ctx, s.transport, s.writeClient().UpdateSubaccountMemberRole, req, decode.Void[authv1.UpdateSubaccountMemberRoleResponse])
	return err
}

func (s *SubAccountsService) InviteMember(ctx context.Context, account AccountScope, subAccountID *string, granteeAccountID, role string) (*models.SubAccountInvite, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return nil, err
	}
	grantee, err := codecs.IDToInt(granteeAccountID, "grantee_account_id")
	if err != nil {
		return nil, err
	}
	protoRole, err := codecs.SubaccountRoleFromLabel(role)
	if err != nil {
		return nil, err
	}
	req := &authv1.InviteSubaccountMemberRequest{SubaccountId: parsed, GranteeAccountId: grantee, Role: protoRole}
	return UnaryAuth(ctx, s.transport, s.writeClient().InviteSubaccountMember, req, decode.InviteSubaccountMemberFromProto)
}

func (s *SubAccountsService) ListInvites(ctx context.Context, direction string) (models.SubAccountInvitesList, error) {
	return UnaryAuth(ctx, s.transport, s.writeClient().ListSubaccountInvites, &authv1.ListSubaccountInvitesRequest{Direction: direction}, decode.SubaccountInvitesListFromProto)
}

func (s *SubAccountsService) RespondInvite(ctx context.Context, inviteID, action string) (*models.SubAccountInvite, error) {
	id, err := codecs.IDToInt(inviteID, "invite_id")
	if err != nil {
		return nil, err
	}
	protoAction, err := codecs.SubaccountInviteActionFromLabel(action)
	if err != nil {
		return nil, err
	}
	req := &authv1.RespondSubaccountInviteRequest{InviteId: id, Action: protoAction}
	return UnaryAuth(ctx, s.transport, s.writeClient().RespondSubaccountInvite, req, decode.RespondSubaccountInviteFromProto)
}

func (s *SubAccountsService) ListActivity(ctx context.Context, account AccountScope, subAccountID *string, limit int, pageToken *string) (models.SubAccountActivityList, error) {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return models.SubAccountActivityList{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	req := &authv1.ListSubaccountEventsRequest{SubaccountId: parsed, Limit: uint32(limit)}
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
