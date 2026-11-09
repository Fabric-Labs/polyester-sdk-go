package decode

import (
	"testing"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// The Devnet account from POLY-9020: root Xiw8QFupu6d, sub-account 2nphEX67UQT.
const (
	invalidationRootID uint64 = 13235591962976289070
	invalidationSubID  uint64 = 771156649032427868
)

func invalidationPayload(t *testing.T, scope *authv1.AccountScopeRef) []byte {
	t.Helper()
	payload, err := proto.Marshal(&authv1.AddressBookViewInvalidated{Scope: scope, ViewRevision: 1004932554200})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// POLY-9020: a sub-account's change names the sub-account and its scope type.
func TestAddressBookInvalidationKeepsTheSubaccountScope(t *testing.T) {
	got, err := AddressBookInvalidationFromBytes(invalidationPayload(t, &authv1.AccountScopeRef{
		ScopeType: authv1.AccountScopeType_SCOPE_SUBACCOUNT, RootAccountId: invalidationRootID, SubaccountId: invalidationSubID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := &models.AccountScope{ScopeType: "SCOPE_SUBACCOUNT", RootAccountID: "Xiw8QFupu6d", SubAccountID: "2nphEX67UQT"}
	if got.Scope == nil || *got.Scope != *want {
		t.Fatalf("scope = %+v, want %+v", got.Scope, want)
	}
	if got.ViewRevision != 1004932554200 {
		t.Fatalf("view revision = %d", got.ViewRevision)
	}
}

// POLY-9020: a root change names the root and no sub-account; no scope is nil.
func TestAddressBookInvalidationRootScopeHasNoSubaccount(t *testing.T) {
	got, err := AddressBookInvalidationFromBytes(invalidationPayload(t, &authv1.AccountScopeRef{
		ScopeType: authv1.AccountScopeType_SCOPE_ROOT, RootAccountId: invalidationRootID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := &models.AccountScope{ScopeType: "SCOPE_ROOT", RootAccountID: "Xiw8QFupu6d"}
	if got.Scope == nil || *got.Scope != *want {
		t.Fatalf("scope = %+v, want %+v", got.Scope, want)
	}
	none, err := AddressBookInvalidationFromBytes(invalidationPayload(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if none.Scope != nil {
		t.Fatalf("missing scope decoded as %+v", none.Scope)
	}
}
