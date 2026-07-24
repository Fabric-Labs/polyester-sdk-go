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

type PoliciesService struct {
	transport        *transport.Factory
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewPoliciesService(factory *transport.Factory, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *PoliciesService {
	return &PoliciesService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *PoliciesService) client() authv1connect.PolicyServiceClient {
	return authv1connect.NewPolicyServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *PoliciesService) ListSubaccountPolicies(ctx context.Context) (models.SubaccountPoliciesList, error) {
	return UnaryAuth(ctx, s.transport, s.client().ListSubaccountPolicies, &authv1.ListSubaccountPoliciesRequest{}, decode.SubaccountPoliciesListFromProto)
}

func (s *PoliciesService) GetSubaccountPolicy(ctx context.Context, policyID string) (*models.SubaccountPolicy, error) {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetSubaccountPolicy, &authv1.GetSubaccountPolicyRequest{PolicyId: id}, decode.GetSubaccountPolicyFromProto)
}

func (s *PoliciesService) CreateSubaccountPolicy(ctx context.Context, account AccountScope, subAccountID *string, opts codecs.SubaccountPolicyWriteOpts) (*models.SubaccountPolicy, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return nil, err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return nil, err
	}
	req, err := codecs.BuildCreateSubaccountPolicyRequest(parsed, opts)
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateSubaccountPolicy, req, decode.CreateSubaccountPolicyFromProto)
}

func (s *PoliciesService) UpdateSubaccountPolicy(ctx context.Context, policyID string, patch codecs.SubaccountPolicyPatch) (*models.SubaccountPolicy, error) {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return nil, err
	}
	req, err := codecs.BuildUpdateSubaccountPolicyRequest(id, patch)
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateSubaccountPolicy, req, decode.UpdateSubaccountPolicyFromProto)
}

func (s *PoliciesService) DeleteSubaccountPolicy(ctx context.Context, policyID string) error {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return err
	}
	_, err = UnaryAuth(ctx, s.transport, s.client().DeleteSubaccountPolicy, &authv1.DeleteSubaccountPolicyRequest{PolicyId: id}, decode.Void[authv1.DeleteSubaccountPolicyResponse])
	return err
}

func (s *PoliciesService) SetSubaccountPolicy(ctx context.Context, account AccountScope, subAccountID *string, policyID string) error {
	parsed, err := s.requireSubaccountID(account, subAccountID)
	if err != nil {
		return err
	}
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return err
	}
	req := &authv1.SetSubaccountPolicyRequest{SubaccountId: parsed, PolicyId: id}
	_, err = UnaryAuth(ctx, s.transport, s.client().SetSubaccountPolicy, req, decode.Void[authv1.SetSubaccountPolicyResponse])
	return err
}

func (s *PoliciesService) ListAPIPolicies(ctx context.Context) (models.ApiPoliciesList, error) {
	return UnaryAuth(ctx, s.transport, s.client().ListApiPolicies, &authv1.ListApiPoliciesRequest{}, decode.ApiPoliciesListFromProto)
}

func (s *PoliciesService) GetAPIPolicy(ctx context.Context, policyID string) (*models.ApiPolicy, error) {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetApiPolicy, &authv1.GetApiPolicyRequest{PolicyId: id}, decode.GetApiPolicyFromProto)
}

func (s *PoliciesService) CreateAPIPolicy(ctx context.Context, opts codecs.ApiPolicyWriteOpts) (*models.ApiPolicy, error) {
	req, err := codecs.BuildCreateApiPolicyRequest(opts)
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateApiPolicy, req, decode.CreateApiPolicyFromProto)
}

func (s *PoliciesService) UpdateAPIPolicy(ctx context.Context, policyID string, patch codecs.ApiPolicyPatch) (*models.ApiPolicy, error) {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return nil, err
	}
	req, err := codecs.BuildUpdateApiPolicyRequest(id, patch)
	if err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateApiPolicy, req, decode.UpdateApiPolicyFromProto)
}

func (s *PoliciesService) DeleteAPIPolicy(ctx context.Context, policyID string) error {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return err
	}
	_, err = UnaryAuth(ctx, s.transport, s.client().DeleteApiPolicy, &authv1.DeleteApiPolicyRequest{PolicyId: id}, decode.Void[authv1.DeleteApiPolicyResponse])
	return err
}

func (s *PoliciesService) SetAPIKeyPolicy(ctx context.Context, keyID, policyID string) error {
	id, err := codecs.IDToInt(policyID, "policy_id")
	if err != nil {
		return err
	}
	req := &authv1.SetApiKeyPolicyRequest{KeyId: keyID, PolicyId: id}
	_, err = UnaryAuth(ctx, s.transport, s.client().SetApiKeyPolicy, req, decode.Void[authv1.SetApiKeyPolicyResponse])
	return err
}

func (s *PoliciesService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.SubaccountPolicy], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:subaccount-policies:{account_id}:proto", accountID, s.defaultAccountID, decode.SubaccountPolicyFromBytes)
}

func (s *PoliciesService) SubscribeSubaccountPolicies(ctx context.Context, accountID any) (*realtime.Subscription[models.SubaccountPolicy], error) {
	return s.Subscribe(ctx, accountID)
}

// SubscribeAPIPolicies subscribes to private API-key policy snapshots for an account.
func (s *PoliciesService) SubscribeAPIPolicies(ctx context.Context, accountID any) (*realtime.Subscription[models.ApiPolicy], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:api-policies:{account_id}:proto", accountID, s.defaultAccountID, decode.ApiPolicyFromBytes)
}

func (s *PoliciesService) requireSubaccountID(account AccountScope, subAccountID *string) (uint64, error) {
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
