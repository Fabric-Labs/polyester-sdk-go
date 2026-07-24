//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

func waitPrivateSubscribeOptional[T any](t *testing.T, label string, sub *realtime.Subscription[T]) {
	t.Helper()
	defer sub.Close()

	select {
	case <-sub.Done():
		t.Skipf("%s closed without publications", label)
	case _, ok := <-sub.Messages():
		if !ok {
			t.Skipf("%s closed without publications", label)
		}
	case <-time.After(5 * time.Second):
		// Subscribe + private-channel auth succeeded; idle channel is OK.
	}
}

func TestPrivateAuthAndLedgerSubscribeConnects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping realtime subscription in short mode")
	}
	client, ok, err := testutil.LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()
	if client.DefaultAccountID == nil || *client.DefaultAccountID == "" {
		t.Skip("POLYESTER_ACCOUNT_ID required for private realtime")
	}
	accountID := *client.DefaultAccountID
	// Many sequential 5s waits; avoid the default 30s RequireLiveClient budget.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	type caseSpec struct {
		label string
		run   func(t *testing.T) error
	}
	cases := []caseSpec{
		{
			label: "api_keys.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.APIKeys.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "api_keys.subscribe", sub)
				return nil
			},
		},
		{
			label: "policies.subscribe_api_policies",
			run: func(t *testing.T) error {
				sub, err := client.Policies.SubscribeAPIPolicies(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "policies.subscribe_api_policies", sub)
				return nil
			},
		},
		{
			label: "policies.subscribe_subaccount_policies",
			run: func(t *testing.T) error {
				sub, err := client.Policies.SubscribeSubaccountPolicies(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "policies.subscribe_subaccount_policies", sub)
				return nil
			},
		},
		{
			label: "sub_accounts.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.SubAccounts.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "sub_accounts.subscribe", sub)
				return nil
			},
		},
		{
			label: "address_book.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.AddressBook.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "address_book.subscribe", sub)
				return nil
			},
		},
		{
			label: "balances.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.Balances.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "balances.subscribe", sub)
				return nil
			},
		},
		{
			label: "transfers.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.Transfers.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "transfers.subscribe", sub)
				return nil
			},
		},
		{
			label: "trades.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.Trades.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "trades.subscribe", sub)
				return nil
			},
		},
		{
			label: "triggers.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.Triggers.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "triggers.subscribe", sub)
				return nil
			},
		},
		{
			label: "orders.subscribe",
			run: func(t *testing.T) error {
				sub, err := client.Orders.Subscribe(ctx, accountID)
				if err != nil {
					return err
				}
				waitPrivateSubscribeOptional(t, "orders.subscribe", sub)
				return nil
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			err := tc.run(t)
			if err == nil {
				return
			}
			if testutil.RouteUnavailable(err) {
				t.Skipf("%s not mounted on devnet: %v", tc.label, err)
			}
			if testutil.JWTSessionOnly(err) {
				t.Skipf("%s requires JWT/session auth: %v", tc.label, err)
			}
			if testutil.DevnetProtoMismatch(err) || testutil.DevnetUnavailable(err) {
				t.Skipf("%s unavailable on devnet: %v", tc.label, err)
			}
			t.Fatalf("%s: %v", tc.label, err)
		})
	}
}
