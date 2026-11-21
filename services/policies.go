package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// PoliciesService exposes private policy realtime streams. The unary policy
// RPCs are unwrapped for now: the reads accept API keys with
// READ_SUBACCOUNT_POLICIES or READ_API_POLICIES, and SetApiKeyPolicy,
// SetSubaccountPolicy and UpdateSubaccountPolicy also accept an admin API key
// with MANAGE_API_KEY_POLICIES or MANAGE_SUBACCOUNT_POLICIES.
type PoliciesService struct {
	defaultAccountID *string
	realtime         RealtimeClient
}

// NewPoliciesService constructs a subscribe-only policies service.
func NewPoliciesService(realtime RealtimeClient, defaultAccountID *string) *PoliciesService {
	return &PoliciesService{realtime: realtime, defaultAccountID: defaultAccountID}
}

// Subscribe streams the account's own sub-account policies, from
// private:auth:subaccount-policies:{account_id}:proto.
//
// It carries the account's own records only. Records the account reaches
// through a delegated sub-account role are published on a separate
// private:auth:subaccount-policies:delegated:{account_id}:proto stream, which only the account's signed-in session may
// subscribe to: an API key is refused there with
// AUTH_SESSION_KIND_NOT_ALLOWED, so this SDK does not expose it. The
// subscription token is issued exactly when the sub-account policy read would answer the
// caller: an API key needs READ_SUBACCOUNT_POLICIES on its policy, and a refusal returns
// that read's own error code (for example AUTH_API_KEY_ACCESS_DENIED).
func (s *PoliciesService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.SubaccountPolicy], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:subaccount-policies:{account_id}:proto", accountID, s.defaultAccountID, decode.SubaccountPolicyFromBytes)
}

// SubscribeSubaccountPolicies is Subscribe: the account's own sub-account
// policies only.
func (s *PoliciesService) SubscribeSubaccountPolicies(ctx context.Context, accountID any) (*realtime.Subscription[models.SubaccountPolicy], error) {
	return s.Subscribe(ctx, accountID)
}

// SubscribeAPIPolicies subscribes to private API-key policy snapshots for an account.
//
// It carries the API-key policies of the account's own keys only. Policies of
// keys on sub-accounts the account administers through a delegated role are
// published on a separate private:auth:api-policies:delegated:{account_id}:proto
// stream, which only the account's signed-in session may subscribe to: an API
// key is refused there with AUTH_SESSION_KIND_NOT_ALLOWED, so this SDK does
// not expose it. The subscription token is issued exactly when the API-key
// policy read would answer the caller: an API key needs READ_API_POLICIES on
// its policy, and a refusal returns that read's own error code (for example
// AUTH_API_KEY_ACCESS_DENIED).
func (s *PoliciesService) SubscribeAPIPolicies(ctx context.Context, accountID any) (*realtime.Subscription[models.ApiPolicy], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:api-policies:{account_id}:proto", accountID, s.defaultAccountID, decode.ApiPolicyFromBytes)
}
