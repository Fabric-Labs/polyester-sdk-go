//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestSubAccountsList(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallRequired(t, "sub_accounts.list", func() (models.SubAccountsList, error) {
		return client.SubAccounts.List(ctx)
	})
	if result.Subaccounts == nil {
		t.Fatal("expected subaccounts list")
	}
}

func TestSubAccountsGetWhenPresent(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "sub_accounts.list", func() (models.SubAccountsList, error) {
		return client.SubAccounts.List(ctx)
	})
	if len(listed.Subaccounts) == 0 {
		t.Skip("no subaccounts on devnet")
	}
	subID := listed.Subaccounts[0].SubaccountID
	result := testutil.CallRequired(t, "sub_accounts.get", func() (models.GetSubaccountResult, error) {
		return client.SubAccounts.Get(ctx, subID, nil, false, true, false, false, false, "")
	})
	if result.Subaccount == nil || result.Subaccount.SubaccountID != subID {
		t.Fatalf("subaccount=%+v want id=%s", result.Subaccount, subID)
	}
}

// TestSubAccountsGetWithBalances reads a sub-account with its balances. A key
// without READ_BALANCES on it is refused with PermissionDenied.
func TestSubAccountsGetWithBalances(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallRequired(t, "sub_accounts.list", func() (models.SubAccountsList, error) {
		return client.SubAccounts.List(ctx)
	})
	if len(listed.Subaccounts) == 0 {
		t.Skip("no subaccounts on devnet")
	}
	subID := listed.Subaccounts[0].SubaccountID
	result := testutil.CallRequired(t, "sub_accounts.get", func() (models.GetSubaccountResult, error) {
		return client.SubAccounts.Get(ctx, nil, &subID, false, false, false, false, true, "")
	})
	if result.Subaccount == nil || result.Subaccount.SubaccountID != subID || result.Balances == nil {
		t.Fatalf("subaccount=%+v balances=%+v", result.Subaccount, result.Balances)
	}
	for _, b := range result.Balances.Balances {
		t.Logf("sub-account %s asset %d trading=%s available=%s reserved=%s funding=%s", subID, b.AssetID, b.Trading, b.Available, b.Reserved, b.Funding)
	}
	t.Logf("sub-account %s: %d balance rows", subID, len(result.Balances.Balances))
}

func TestSubAccountsListMembersOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	listed := testutil.CallOptional(t, "sub_accounts.list", func() (models.SubAccountsList, error) {
		return client.SubAccounts.List(ctx)
	})
	if len(listed.Subaccounts) == 0 {
		t.Skip("no subaccounts on devnet")
	}
	subID := listed.Subaccounts[0].SubaccountID
	_ = testutil.CallOptional(t, "sub_accounts.list_members", func() (models.SubAccountMembersList, error) {
		return client.SubAccounts.ListMembers(ctx, subID, nil)
	})
}

func TestSubAccountsListInvitesOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "sub_accounts.list_invites", func() (models.SubAccountInvitesList, error) {
		return client.SubAccounts.ListInvites(ctx, "")
	})
}

func TestAddressBookGetViewOptional(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	view := testutil.CallOptional(t, "address_book.get_view", func() (models.AddressBookView, error) {
		return client.AddressBook.GetView(ctx, nil, nil, 10, 0)
	})
	if len(view.Books) == 0 && view.Entries == nil {
		t.Skip("address book view empty")
	}
}
