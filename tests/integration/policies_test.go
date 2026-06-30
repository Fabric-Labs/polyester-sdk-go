//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestPoliciesListSubaccountPolicies(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "policies.list_subaccount_policies", func() (models.SubaccountPoliciesList, error) {
		return client.Policies.ListSubaccountPolicies(ctx)
	})
	if result.Policies == nil {
		t.Fatal("expected policies list")
	}
}

func TestPoliciesListAPIPolicies(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "policies.list_api_policies", func() (models.ApiPoliciesList, error) {
		return client.Policies.ListAPIPolicies(ctx)
	})
	if result.Policies == nil {
		t.Fatal("expected api policies list")
	}
}

func TestPoliciesGetSubaccountPolicyWhenPresent(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "policies.list_subaccount_policies", func() (models.SubaccountPoliciesList, error) {
		return client.Policies.ListSubaccountPolicies(ctx)
	})
	if len(listed.Policies) == 0 {
		t.Skip("no subaccount policies on devnet")
	}
	policyID := listed.Policies[0].PolicyID
	policy := testutil.CallRequired(t, "policies.get_subaccount_policy", func() (*models.SubaccountPolicy, error) {
		return client.Policies.GetSubaccountPolicy(ctx, policyID)
	})
	if policy == nil || policy.PolicyID != policyID {
		t.Fatalf("policy=%+v want id=%s", policy, policyID)
	}
}

func TestPoliciesGetAPIPolicyWhenPresent(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "policies.list_api_policies", func() (models.ApiPoliciesList, error) {
		return client.Policies.ListAPIPolicies(ctx)
	})
	if len(listed.Policies) == 0 {
		t.Skip("no api policies on devnet")
	}
	policyID := listed.Policies[0].PolicyID
	policy := testutil.CallRequired(t, "policies.get_api_policy", func() (*models.ApiPolicy, error) {
		return client.Policies.GetAPIPolicy(ctx, policyID)
	})
	if policy == nil || policy.PolicyID != policyID {
		t.Fatalf("policy=%+v want id=%s", policy, policyID)
	}
}
