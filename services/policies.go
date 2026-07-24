package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// PoliciesService exposes private policy realtime streams.
// Unary policy RPCs require JWT/session auth and are not wrapped in this API-key SDK.
type PoliciesService struct {
	defaultAccountID *string
	realtime         RealtimeClient
}

// NewPoliciesService constructs a subscribe-only policies service.
func NewPoliciesService(realtime RealtimeClient, defaultAccountID *string) *PoliciesService {
	return &PoliciesService{realtime: realtime, defaultAccountID: defaultAccountID}
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
