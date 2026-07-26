//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// Permission fixtures (F-24 / B6b): each subscribe group needs the matching
// API-key scope. Missing permission returns structured Auth/403 and soft-skips
// (fails closed under POLYESTER_TEST_STRICT_LIVE=1):
// - address_book.subscribe → address-book read
// - transfers.subscribe → transfer:read
// - orders / trades / triggers → trading read
// - balances.subscribe → ledger read
// - api_keys / policies / sub_accounts → auth admin read

func waitPrivateSubscribeOptional[T any](t *testing.T, label string, sub *realtime.Subscription[T]) {
	t.Helper()
	defer sub.Close()

	select {
	case <-sub.Done():
		if err := sub.Err(); err != nil {
			t.Fatalf("%s failed: %v", label, err)
		}
		testutil.SoftSkipf(t, "%s closed without publications", label)
	case _, ok := <-sub.Messages():
		if !ok {
			if err := sub.Err(); err != nil {
				t.Fatalf("%s failed: %v", label, err)
			}
			testutil.SoftSkipf(t, "%s closed without publications", label)
		}
	case <-time.After(5 * time.Second):
		// Subscribe already waited for private-channel auth handshake; idle is OK.
		if err := sub.Err(); err != nil {
			t.Fatalf("%s failed after handshake: %v", label, err)
		}
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
		testutil.SoftSkip(t, "POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()
	if client.DefaultAccountID == nil || *client.DefaultAccountID == "" {
		testutil.SoftSkip(t, "POLYESTER_ACCOUNT_ID required for private realtime")
	}
	accountID := *client.DefaultAccountID
	// Many sequential 5s waits; avoid the default 30s RequireLiveClient budget.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	type caseSpec struct {
		label              string
		requiredPermission string
		run                func(t *testing.T) error
	}
	cases := []caseSpec{
		{
			label:              "api_keys.subscribe",
			requiredPermission: "auth admin read",
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
			label:              "policies.subscribe_api_policies",
			requiredPermission: "auth admin read",
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
			label:              "policies.subscribe_subaccount_policies",
			requiredPermission: "auth admin read",
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
			label:              "sub_accounts.subscribe",
			requiredPermission: "auth admin read",
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
			label:              "address_book.subscribe",
			requiredPermission: "address-book read",
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
			label:              "balances.subscribe",
			requiredPermission: "ledger read",
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
			label:              "transfers.subscribe",
			requiredPermission: "transfer:read",
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
			label:              "trades.subscribe",
			requiredPermission: "trading read",
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
			label:              "triggers.subscribe",
			requiredPermission: "trading read",
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
			label:              "orders.subscribe",
			requiredPermission: "trading read",
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
			if testutil.IsPermissionDenied(err) {
				testutil.SoftSkipf(t, "%s missing required API-key permission (%s; declare fixture scopes): %v",
					tc.label, tc.requiredPermission, err)
			}
			if testutil.RouteUnavailable(err) {
				testutil.SoftSkipf(t, "%s not mounted on devnet: %v", tc.label, err)
			}
			if testutil.JWTSessionOnly(err) {
				testutil.SoftSkipf(t, "%s requires JWT/session auth: %v", tc.label, err)
			}
			if testutil.DevnetProtoMismatch(err) || testutil.DevnetUnavailable(err) {
				testutil.SoftSkipf(t, "%s unavailable on devnet: %v", tc.label, err)
			}
			t.Fatalf("%s: %v", tc.label, err)
		})
	}
}
