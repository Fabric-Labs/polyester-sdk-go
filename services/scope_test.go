package services_test

import (
	stderrors "errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

func TestScopedSubAccountIDMainUsesDefault(t *testing.T) {
	defaultSub := "sub-123"
	got, err := services.ScopedSubAccountID("main", nil, &defaultSub)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != defaultSub {
		t.Fatalf("got=%v want=%s", got, defaultSub)
	}
	got, err = services.ScopedSubAccountID("active", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got=%v want nil", got)
	}
	empty := ""
	got, err = services.ScopedSubAccountID("main", nil, &empty)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got=%v want nil for empty default subaccount", got)
	}
}

func TestScopedSubAccountIDDict(t *testing.T) {
	got, err := services.ScopedSubAccountID(map[string]string{"subaccountId": "abc"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != "abc" {
		t.Fatalf("got=%v want abc", got)
	}
}

func TestScopedSubAccountIDRejectsBoth(t *testing.T) {
	legacy := "x"
	_, err := services.ScopedSubAccountID("main", &legacy, nil)
	var validation *sdkerrors.ValidationError
	if err == nil || !stderrors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestScopedSubAccountIDPrefersAccountScope(t *testing.T) {
	defaultSub := "default"
	got, err := services.ScopedSubAccountID(map[string]string{"subaccountId": "scoped"}, nil, &defaultSub)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != "scoped" {
		t.Fatalf("got=%v want scoped", got)
	}
	legacy := "legacy"
	got, err = services.ScopedSubAccountID(nil, &legacy, &defaultSub)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != legacy {
		t.Fatalf("got=%v want legacy", got)
	}
}

func TestScopedSubAccountMixinIgnoresTypedNilSubAccountID(t *testing.T) {
	defaultSub := "default-sub"
	scoped := services.ScopedSubAccount{DefaultSubAccountID: &defaultSub}
	var sub *string
	got, err := scoped.ResolveSubAccountID(sub, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != defaultSub {
		t.Fatalf("got=%v want=%s", got, defaultSub)
	}
}

func TestScopedSubAccountMixinOnService(t *testing.T) {
	defaultSub := "default-sub"
	scoped := services.ScopedSubAccount{DefaultSubAccountID: &defaultSub}
	got, err := scoped.ResolveSubAccountID(nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != defaultSub {
		t.Fatalf("got=%v want=%s", got, defaultSub)
	}
	got, err = scoped.ResolveSubAccountID(nil, map[string]string{"subaccountId": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != "x" {
		t.Fatalf("got=%v want x", got)
	}
}
